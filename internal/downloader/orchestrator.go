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

	// Create or open file for resume (keeps existing data if file exists and has correct size)
	writer, err := filewriter.NewWriterForResume(o.outputPath, int64(o.meta.Length), o.meta.PieceLength)
	if err != nil {
		return fmt.Errorf("failed to create file writer: %w", err)
	}
	defer writer.Close()

	totalPieces := o.meta.NumPieces()
	downloadedCount := 0
	const minIntervalSec = 5   // minimum delay between re-announces (tracker interval can be 0 or very small)
	const retryDelaySec = 30   // delay between retry rounds when using same peers (no tracker call)
	remaining := make(map[int]struct{})
	for i := 0; i < totalPieces; i++ {
		remaining[i] = struct{}{}
	}

	// Idempotency: verify already-downloaded pieces on disk and skip them
	for i := 0; i < totalPieces; i++ {
		pieceLen := o.meta.PieceSize(i)
		offset := int64(i) * int64(o.meta.PieceLength)
		data, err := writer.ReadAt(offset, pieceLen)
		if err != nil {
			continue // file too short or read error; will re-download this piece
		}
		if err := VerifyPiece(data, o.meta.Pieces[i]); err != nil {
			continue // hash mismatch; will re-download this piece
		}
		delete(remaining, i)
		progressTracker.AddPiece(pieceLen)
	}
	if len(remaining) < totalPieces {
		fmt.Printf("Resuming: %d pieces already present, %d to download.\n", totalPieces-len(remaining), len(remaining))
	}

	peers := o.peers
	announceIntervalSec := minIntervalSec
	lastAnnounceTime := time.Now() // treat initial peer list as just announced (respect interval from first re-announce)
	for round := 0; ; round++ {
		if len(remaining) == 0 {
			break
		}
		if round > 0 {
			// Short delay between retry rounds (we retry with same peers often; only re-announce when tracker allows).
			time.Sleep(time.Duration(retryDelaySec) * time.Second)

			// Re-announce to tracker only when the tracker's interval has elapsed (avoids rate limits / bans).
			if time.Since(lastAnnounceTime) >= time.Duration(announceIntervalSec)*time.Second {
				var err error
				peers, announceIntervalSec, err = tracker.GetPeersWithPeerID(o.meta, o.peerID)
				lastAnnounceTime = time.Now()
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
			} else {
				secUntilAnnounce := announceIntervalSec - int(time.Since(lastAnnounceTime).Seconds())
				if secUntilAnnounce < 0 {
					secUntilAnnounce = 0
				}
				fmt.Printf("\nRetry round %d with same peers (re-announce in %ds). %d pieces left\n", round+1, secUntilAnnounce, len(remaining))
			}
		}

		// FIX: Always create pieceIndices from remaining (including round 0)
		// This prevents downloading pieces that are already on disk
		pieceIndices := make([]int, 0, len(remaining))
		for i := range remaining {
			pieceIndices = append(pieceIndices, i)
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

			err := writer.WritePiece(result.Index, result.Data)
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
	completedCount := totalPieces - len(remaining)
	if completedCount < totalPieces {
		return fmt.Errorf("incomplete download: got %d/%d pieces (missing %d)", completedCount, totalPieces, len(remaining))
	}

	// Verify file size
	if err := writer.VerifyFileSize(); err != nil {
		return fmt.Errorf("file size verification failed: %w", err)
	}

	fmt.Printf("\n✓ Download complete!\n")
	fmt.Printf("File saved to: %s\n", o.outputPath)

	return nil
}
