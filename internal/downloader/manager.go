package downloader

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/yourusername/torrent-client/internal/torrent"
	"github.com/yourusername/torrent-client/internal/tracker"
)

// Piece represents a single piece to be downloaded
type Piece struct {
	Index  int      // Piece index
	Hash   []byte   // Expected SHA1 hash
	Length int      // Length of this piece in bytes
	Data   []byte   // Downloaded data (filled by workers)
}

// PieceWork represents work to be done by a worker
type PieceWork struct {
	Index  int
	Hash   []byte
	Length int
}

// PieceResult represents the result of downloading a piece
type PieceResult struct {
	Index int
	Data  []byte
	Err   error
}

// Manager manages the download process with worker pool
type Manager struct {
	meta         *torrent.TorrentMeta
	peers        []tracker.Peer
	peerID       [20]byte
	workQueue    chan PieceWork
	resultQueue  chan PieceResult
	pieces       []Piece
	numWorkers   int
	downloaded   int32 // Atomic counter for downloaded pieces
	mu           sync.Mutex
}

// NewManager creates a new download manager
func NewManager(meta *torrent.TorrentMeta, peers []tracker.Peer, peerID [20]byte, numWorkers int) *Manager {
	// Initialize pieces
	pieces := make([]Piece, meta.NumPieces())
	for i := 0; i < meta.NumPieces(); i++ {
		pieces[i] = Piece{
			Index:  i,
			Hash:   meta.Pieces[i],
			Length: meta.PieceSize(i),
		}
	}

	return &Manager{
		meta:        meta,
		peers:       peers,
		peerID:      peerID,
		workQueue:   make(chan PieceWork, meta.NumPieces()),
		resultQueue: make(chan PieceResult),
		pieces:      pieces,
		numWorkers:  numWorkers,
		downloaded:  0,
	}
}

// Start begins the download process with worker pool
func (m *Manager) Start() error {
	// Fill work queue with all pieces
	for i := 0; i < len(m.pieces); i++ {
		m.workQueue <- PieceWork{
			Index:  m.pieces[i].Index,
			Hash:   m.pieces[i].Hash,
			Length: m.pieces[i].Length,
		}
	}
	close(m.workQueue) // No more work will be added

	// Start workers using goroutines
	var wg sync.WaitGroup
	for i := 0; i < m.numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			m.runWorker(workerID)
		}(i)
	}

	// Close result queue when all workers are done
	go func() {
		wg.Wait()
		close(m.resultQueue)
	}()

	return nil
}

// CollectResults collects downloaded pieces from result queue
func (m *Manager) CollectResults() ([]Piece, error) {
	totalPieces := len(m.pieces)
	downloaded := make(map[int]bool)

	for result := range m.resultQueue {
		if result.Err != nil {
			fmt.Printf("Error downloading piece %d: %v\n", result.Index, result.Err)
			// In a real implementation, we'd retry failed pieces
			continue
		}

		// Store the piece data
		m.mu.Lock()
		m.pieces[result.Index].Data = result.Data
		m.mu.Unlock()

		downloaded[result.Index] = true
		atomic.AddInt32(&m.downloaded, 1)

		// Progress update
		progress := float64(len(downloaded)) / float64(totalPieces) * 100
		fmt.Printf("Progress: %.1f%% (%d/%d pieces)\n", progress, len(downloaded), totalPieces)
	}

	// Check if we got all pieces
	if len(downloaded) != totalPieces {
		return nil, fmt.Errorf("incomplete download: got %d/%d pieces", len(downloaded), totalPieces)
	}

	return m.pieces, nil
}

// GetDownloadedCount returns the number of downloaded pieces (thread-safe)
func (m *Manager) GetDownloadedCount() int {
	return int(atomic.LoadInt32(&m.downloaded))
}

// VerifyPiece verifies a piece against its hash
func VerifyPiece(data []byte, expectedHash []byte) error {
	hash := sha1.Sum(data)
	if !bytes.Equal(hash[:], expectedHash) {
		return fmt.Errorf("hash mismatch: expected %x, got %x", expectedHash, hash[:])
	}
	return nil
}

// AssembleFile assembles all pieces into the final file data
func AssembleFile(pieces []Piece, totalLength int) ([]byte, error) {
	fileData := make([]byte, totalLength)
	offset := 0

	for i, piece := range pieces {
		if piece.Data == nil {
			return nil, fmt.Errorf("piece %d has no data", i)
		}

		// Verify piece hash
		if err := VerifyPiece(piece.Data, piece.Hash); err != nil {
			return nil, fmt.Errorf("piece %d verification failed: %w", i, err)
		}

		// Copy piece data to file
		copy(fileData[offset:], piece.Data)
		offset += len(piece.Data)
	}

	return fileData, nil
}
