package downloader

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"sync"

	"github.com/yourusername/torrent-client/internal/torrent"
	"github.com/yourusername/torrent-client/internal/tracker"
)

// PieceWork describes one piece to be downloaded by a worker
type PieceWork struct {
	Index  int
	Hash   []byte
	Length int
}

// PieceResult is what a worker sends back after attempting a piece
type PieceResult struct {
	Index int
	Data  []byte
	Err   error
}

// Manager manages the download process with a worker pool
type Manager struct {
	meta        *torrent.TorrentMeta
	peers       []tracker.Peer
	peerID      [20]byte
	work        []PieceWork
	workQueue   chan PieceWork
	resultQueue chan PieceResult
	numWorkers  int
}

// NewManager creates a new Manager for the given piece indices.
// It builds PieceWork entries directly — no intermediate Piece type.
func NewManager(meta *torrent.TorrentMeta, peers []tracker.Peer, peerID [20]byte, numWorkers int, pieceIndices []int) *Manager {
	work := make([]PieceWork, len(pieceIndices))
	for i, idx := range pieceIndices {
		work[i] = PieceWork{
			Index:  idx,
			Hash:   meta.Pieces[idx],
			Length: meta.PieceSize(idx),
		}
	}

	queueCap := len(work)
	if queueCap < numWorkers*2 {
		queueCap = numWorkers * 2
	}

	return &Manager{
		meta:        meta,
		peers:       peers,
		peerID:      peerID,
		work:        work,
		workQueue:   make(chan PieceWork, queueCap),
		resultQueue: make(chan PieceResult),
		numWorkers:  numWorkers,
	}
}

// Start fills the work queue, spawns workers, and arranges for resultQueue
// to be closed when all workers finish.
func (m *Manager) Start() {
	for _, w := range m.work {
		m.workQueue <- w
	}
	close(m.workQueue)

	var wg sync.WaitGroup
	for i := 0; i < m.numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.runWorker()
		}()
	}

	go func() {
		wg.Wait()
		close(m.resultQueue)
	}()
}

// VerifyPiece checks that the SHA1 of data matches the expected hash
func VerifyPiece(data []byte, expectedHash []byte) error {
	hash := sha1.Sum(data)
	if !bytes.Equal(hash[:], expectedHash) {
		return fmt.Errorf("hash mismatch: expected %x, got %x", expectedHash, hash[:])
	}
	return nil
}
