package downloader

import (
	"fmt"
	"time"

	"github.com/yourusername/torrent-client/internal/filewriter"
	"github.com/yourusername/torrent-client/internal/progress"
	"github.com/yourusername/torrent-client/internal/torrent"
	"github.com/yourusername/torrent-client/internal/tracker"
)

// Orchestrator coordinates the entire download process
type Orchestrator struct {
	meta       *torrent.TorrentMeta
	peers      []tracker.Peer
	peerID     [20]byte
	outputPath string
	numWorkers int
}

// NewOrchestrator creates a new download orchestrator
func NewOrchestrator(
	meta *torrent.TorrentMeta,
	peers []tracker.Peer,
	peerID [20]byte,
	outputPath string,
	numWorkers int,
) *Orchestrator {
	return &Orchestrator{
		meta:       meta,
		peers:      peers,
		peerID:     peerID,
		outputPath: outputPath,
		numWorkers: numWorkers,
	}
}

// Download orchestrates the entire download process
func (o *Orchestrator) Download() error {
	fmt.Printf("\n=== Starting Download ===\n")
	fmt.Printf("File: %s\n", o.meta.Name)
	fmt.Printf("Size: %.2f MB (%d bytes)\n", float64(o.meta.Length)/(1024*1024), o.meta.Length)
	fmt.Printf("Pieces: %d\n", o.meta.NumPieces())
	fmt.Printf("Workers: %d\n", o.numWorkers)
	fmt.Printf("Peers: %d\n", len(o.peers))
	fmt.Println()

	// Create progress tracker
	progressTracker := progress.NewTracker(o.meta.NumPieces(), int64(o.meta.Length))

	// Create file writer
	writer, err := filewriter.NewWriter(o.outputPath, int64(o.meta.Length))
	if err != nil {
		return fmt.Errorf("failed to create file writer: %w", err)
	}
	defer writer.Close()

	totalPieces := o.meta.NumPieces()
	downloadedCount := 0
	const minIntervalSec = 5   // minimum delay between re-announces (tracker interval can be 0 or very small)
	const retryDelaySec = 30   // delay before re-announce when retrying missing pieces (avoids 15-min waits)
	remaining := make(map[int]struct{})
	for i := 0; i < totalPieces; i++ {
		remaining[i] = struct{}{}
	}

	peers := o.peers
	announceIntervalSec := minIntervalSec
	for round := 0; ; round++ {
		if len(remaining) == 0 {
			break
		}
		if round > 0 {
			// Use short delay for retries so we don't wait the full tracker interval (e.g. 900s) between rounds.
			delaySec := retryDelaySec
			if delaySec > announceIntervalSec {
				delaySec = announceIntervalSec
			}
			time.Sleep(time.Duration(delaySec) * time.Second)
			var err error
			peers, announceIntervalSec, err = tracker.GetPeersWithPeerID(o.meta, o.peerID)
			if err != nil {
				fmt.Printf("\nRe-announce failed: %v; using previous peer list. Retry round %d: %d pieces left\n", err, round+1, len(remaining))
			} else if len(peers) == 0 {
				fmt.Printf("\nRe-announce returned 0 peers; using previous list. Retry round %d: %d pieces left\n", round+1, len(remaining))
			} else {
				fmt.Printf("\nRe-announced: %d peers (next in %ds). Retry round %d: %d pieces left\n", len(peers), announceIntervalSec, round+1, len(remaining))
			}
			if announceIntervalSec < minIntervalSec {
				announceIntervalSec = minIntervalSec
			}
		}

		var pieceIndices []int
		if round > 0 {
			pieceIndices = make([]int, 0, len(remaining))
			for i := range remaining {
				pieceIndices = append(pieceIndices, i)
			}
		}

		manager := NewManager(o.meta, peers, o.peerID, o.numWorkers, pieceIndices)
		if err := manager.Start(); err != nil {
			return fmt.Errorf("failed to start download manager: %w", err)
		}

		if round == 0 {
			fmt.Println("Downloading...")
		}

		for result := range manager.resultQueue {
			if result.Err != nil {
				continue
			}

			delete(remaining, result.Index)

			err := writer.WritePiece(result.Index, len(result.Data), result.Data)
			if err != nil {
				return fmt.Errorf("failed to write piece %d: %w", result.Index, err)
			}

			progressTracker.AddPiece(len(result.Data))
			downloadedCount++

			stats := progressTracker.GetStats()
			fmt.Printf("\r%s", stats.FormatSimple())
		}
	}

	fmt.Println()
	if downloadedCount < totalPieces {
		return fmt.Errorf("incomplete download: got %d/%d pieces (missing %d)", downloadedCount, totalPieces, totalPieces-downloadedCount)
	}

	// Verify file size
	if err := writer.VerifyFileSize(); err != nil {
		return fmt.Errorf("file size verification failed: %w", err)
	}

	fmt.Printf("\n✓ Download complete!\n")
	fmt.Printf("File saved to: %s\n", o.outputPath)

	return nil
}
