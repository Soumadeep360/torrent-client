package downloader

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"sync"

	"github.com/yourusername/torrent-client/internal/torrent"
	"github.com/yourusername/torrent-client/internal/tracker"
)

// Piece represents a single piece to be downloaded
type Piece struct {
	Index  int    // Piece index
	Hash   []byte // Expected SHA1 hash
	Length int    // Length of this piece in bytes
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
	pieceIndices []int // indices to download; nil means all
}

// NewManager creates a new download manager. If pieceIndices is non-nil, only those
// piece indices are queued (for retries); otherwise all pieces are queued.
func NewManager(meta *torrent.TorrentMeta, peers []tracker.Peer, peerID [20]byte, numWorkers int, pieceIndices []int) *Manager {
	// Initialize pieces
	pieces := make([]Piece, meta.NumPieces())
	for i := 0; i < meta.NumPieces(); i++ {
		pieces[i] = Piece{
			Index:  i,
			Hash:   meta.Pieces[i],
			Length: meta.PieceSize(i),
		}
	}

	queueCap := meta.NumPieces()
	if pieceIndices != nil {
		queueCap = len(pieceIndices)
	}
	if queueCap < numWorkers*2 {
		queueCap = numWorkers * 2
	}

	return &Manager{
		meta:         meta,
		peers:        peers,
		peerID:       peerID,
		workQueue:    make(chan PieceWork, queueCap),
		resultQueue:  make(chan PieceResult),
		pieces:       pieces,
		numWorkers:   numWorkers,
		pieceIndices: pieceIndices,
	}
}

// Start begins the download process with worker pool
func (m *Manager) Start() error {
	indices := m.pieceIndices
	if indices == nil {
		for i := 0; i < len(m.pieces); i++ {
			m.workQueue <- PieceWork{
				Index:  m.pieces[i].Index,
				Hash:   m.pieces[i].Hash,
				Length: m.pieces[i].Length,
			}
		}
	} else {
		for _, i := range indices {
			m.workQueue <- PieceWork{
				Index:  m.pieces[i].Index,
				Hash:   m.pieces[i].Hash,
				Length: m.pieces[i].Length,
			}
		}
	}
	close(m.workQueue)

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

// VerifyPiece verifies a piece against its hash
func VerifyPiece(data []byte, expectedHash []byte) error {
	hash := sha1.Sum(data)
	if !bytes.Equal(hash[:], expectedHash) {
		return fmt.Errorf("hash mismatch: expected %x, got %x", expectedHash, hash[:])
	}
	return nil
}

