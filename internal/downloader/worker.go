package downloader

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"time"

	"github.com/yourusername/torrent-client/internal/peer"
	"github.com/yourusername/torrent-client/internal/tracker"
)

const (
	// Block size for requests (16KB is standard)
	blockSize = 16384

	// Maximum number of retries per piece
	maxRetries = 3
)

// runWorker is executed by each worker goroutine
func (m *Manager) runWorker(workerID int) {
	// Each worker processes pieces from the work queue
	for work := range m.workQueue {
		// Try to download this piece
		data, err := m.downloadPiece(work)

		// Send result to result queue
		m.resultQueue <- PieceResult{
			Index: work.Index,
			Data:  data,
			Err:   err,
		}
	}
}

// downloadPiece downloads a single piece from any available peer
func (m *Manager) downloadPiece(work PieceWork) ([]byte, error) {
	var lastErr error

	// Try multiple times with different peers
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Pick a random peer
		if len(m.peers) == 0 {
			return nil, fmt.Errorf("no peers available")
		}

		peerIndex := rand.Intn(len(m.peers))
		selectedPeer := m.peers[peerIndex]

		// Try to download from this peer
		data, err := m.downloadPieceFromPeer(selectedPeer, work)
		if err != nil {
			lastErr = err
			continue // Try another peer
		}

		// Verify the piece hash
		if err := VerifyPiece(data, work.Hash); err != nil {
			lastErr = fmt.Errorf("piece verification failed: %w", err)
			continue
		}

		// Success!
		return data, nil
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

// downloadPieceFromPeer downloads a piece from a specific peer
func (m *Manager) downloadPieceFromPeer(peerAddr tracker.Peer, work PieceWork) ([]byte, error) {
	// Connect to peer and perform handshake
	conn, err := peer.ConnectAndHandshake(peerAddr, m.meta.InfoHash, m.peerID)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()

	// Send interested message
	if err := peer.SendInterested(conn.Conn); err != nil {
		return nil, fmt.Errorf("failed to send interested: %w", err)
	}

	// Wait for unchoke message
	unchoked := false
	for i := 0; i < 10; i++ { // Try reading a few messages
		msg, err := peer.ReadMessage(conn.Conn)
		if err != nil {
			return nil, fmt.Errorf("failed to read message: %w", err)
		}

		if msg == nil {
			continue // Keep-alive
		}

		// Check for unchoke (ID = 1)
		if msg.ID == 1 {
			unchoked = true
			break
		}

		// Handle bitfield (ID = 5) - just acknowledge it
		if msg.ID == 5 {
			// In a real implementation, we'd check if peer has this piece
			continue
		}
	}

	if !unchoked {
		return nil, fmt.Errorf("peer did not unchoke us")
	}

	// Download piece in blocks
	pieceData := make([]byte, work.Length)
	downloaded := 0

	for downloaded < work.Length {
		// Calculate block size for this request
		remainingBytes := work.Length - downloaded
		requestSize := blockSize
		if remainingBytes < blockSize {
			requestSize = remainingBytes
		}

		// Request block
		err := peer.SendRequest(
			conn.Conn,
			uint32(work.Index),
			uint32(downloaded),
			uint32(requestSize),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}

		// Wait for piece message (ID = 7)
		msg, err := peer.ReadMessage(conn.Conn)
		if err != nil {
			return nil, fmt.Errorf("failed to read piece message: %w", err)
		}

		if msg == nil {
			return nil, fmt.Errorf("unexpected keep-alive during download")
		}

		if msg.ID != 7 {
			return nil, fmt.Errorf("expected piece message (7), got %d", msg.ID)
		}

		// Parse piece message: <index><begin><block>
		if len(msg.Payload) < 8 {
			return nil, fmt.Errorf("piece message too short: %d bytes", len(msg.Payload))
		}

		receivedIndex := binary.BigEndian.Uint32(msg.Payload[0:4])
		receivedBegin := binary.BigEndian.Uint32(msg.Payload[4:8])
		blockData := msg.Payload[8:]

		// Verify this is the block we requested
		if int(receivedIndex) != work.Index {
			return nil, fmt.Errorf("wrong piece index: expected %d, got %d", work.Index, receivedIndex)
		}

		if int(receivedBegin) != downloaded {
			return nil, fmt.Errorf("wrong block offset: expected %d, got %d", downloaded, receivedBegin)
		}

		// Copy block data
		copy(pieceData[downloaded:], blockData)
		downloaded += len(blockData)
	}

	return pieceData, nil
}

func init() {
	// Seed random number generator for peer selection
	rand.Seed(time.Now().UnixNano())
}
