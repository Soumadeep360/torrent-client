package downloader

import (
	"fmt"

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

	// Create download manager
	manager := NewManager(o.meta, o.peers, o.peerID, o.numWorkers)

	// Start worker pool
	if err := manager.Start(); err != nil {
		return fmt.Errorf("failed to start download manager: %w", err)
	}

	// Create file writer
	writer, err := filewriter.NewWriter(o.outputPath, int64(o.meta.Length))
	if err != nil {
		return fmt.Errorf("failed to create file writer: %w", err)
	}
	defer writer.Close()

	// Collect results and write pieces
	fmt.Println("Downloading...")
	totalPieces := o.meta.NumPieces()
	downloadedCount := 0

	for result := range manager.resultQueue {
		if result.Err != nil {
			fmt.Printf("Warning: piece %d failed: %v\n", result.Index, result.Err)
			continue
		}

		// Write piece to file
		err := writer.WritePiece(result.Index, len(result.Data), result.Data)
		if err != nil {
			return fmt.Errorf("failed to write piece %d: %w", result.Index, err)
		}

		// Update progress
		progressTracker.AddPiece(len(result.Data))
		downloadedCount++

		// Display progress
		stats := progressTracker.GetStats()
		fmt.Printf("\r%s", stats.FormatSimple())

		// Check if complete
		if downloadedCount >= totalPieces {
			fmt.Println() // New line
			break
		}
	}

	// Only report success if we actually got all pieces
	if downloadedCount < totalPieces {
		return fmt.Errorf("incomplete download: got %d/%d pieces (missing %d). Some peers may be unreachable; try again later", downloadedCount, totalPieces, totalPieces-downloadedCount)
	}

	// Verify file size
	if err := writer.VerifyFileSize(); err != nil {
		return fmt.Errorf("file size verification failed: %w", err)
	}

	fmt.Printf("\n✓ Download complete!\n")
	fmt.Printf("File saved to: %s\n", o.outputPath)

	return nil
}
