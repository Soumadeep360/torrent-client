package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/yourusername/torrent-client/internal/downloader"
	"github.com/yourusername/torrent-client/internal/torrent"
	"github.com/yourusername/torrent-client/internal/tracker"
)

const (
	defaultWorkers = 50 // Default number of concurrent workers (override with -workers N)
)

func main() {
	// Parse command-line flags
	numWorkers := flag.Int("workers", defaultWorkers, "Number of concurrent download workers")
	outputDir := flag.String("output", ".", "Output directory for downloaded file")
	verbose := flag.Bool("verbose", false, "Enable verbose output")
	flag.Parse()

	// Check arguments
	if flag.NArg() < 1 {
		printUsage()
		os.Exit(1)
	}

	torrentPath := flag.Arg(0) // first non-flag argument.

	// Banner
	printBanner()

	// Step 1: Parse torrent file
	fmt.Println("=== Step 1: Parsing Torrent File ===")
	fmt.Printf("Reading: %s\n", torrentPath)

	meta, err := torrent.ParseTorrentFile(torrentPath)
	if err != nil {
		log.Fatalf("Failed to parse torrent file: %v", err)
	}

	printTorrentInfo(meta, *verbose)
	fmt.Println()

	// Step 2: Contact tracker (use same peer ID for tracker and download)
	fmt.Println("=== Step 2: Contacting Tracker ===")
	fmt.Printf("Tracker: %s\n", meta.Announce)

	peerID := generatePeerID()
	peers, _, err := tracker.GetPeersWithPeerID(meta, peerID)
	if err != nil {
		log.Fatalf("Failed to get peers from tracker: %v", err)
	}

	fmt.Printf("✓ Found %d peers\n", len(peers))
	if *verbose {
		printPeerList(peers, 5)
	}
	fmt.Println()

	if len(peers) == 0 {
		log.Fatal("No peers available. Cannot download.")
	}

	// Step 3: Prepare download
	fmt.Println("=== Step 3: Preparing Download ===")
	outputPath := filepath.Join(*outputDir, meta.Name)

	fmt.Printf("Workers: %d\n", *numWorkers)
	fmt.Printf("Output: %s\n", outputPath)
	fmt.Println()

	// Step 4: Start download
	orchestrator := downloader.NewOrchestrator(
		meta,
		peers,
		peerID,
		outputPath,
		*numWorkers,
	)

	if err := orchestrator.Download(); err != nil {
		log.Fatalf("Download failed: %v", err)
	}

	// Success
	fmt.Println("\n=== Download Complete ===")
	fmt.Printf("✓ File: %s\n", meta.Name)
	fmt.Printf("✓ Size: %.2f MB\n", float64(meta.Length)/(1024*1024))
	fmt.Printf("✓ Saved to: %s\n", outputPath)
	fmt.Println("\nThank you for using Go Torrent Client!")
}

// generatePeerID generates a 20-byte peer ID
func generatePeerID() [20]byte {
	var peerID [20]byte
	copy(peerID[:], "-GO0001-")
	rand.Read(peerID[8:])
	return peerID
}

// printBanner prints the application banner
func printBanner() {
	fmt.Println("╔═══════════════════════════════════════╗")
	fmt.Println("║     Go BitTorrent Client v1.0         ║")
	fmt.Println("║   Demonstrating Go Concurrency        ║")
	fmt.Println("╚═══════════════════════════════════════╝")
	fmt.Println()
}

// printUsage prints usage information
func printUsage() {
	fmt.Println("Usage: torrent-client [OPTIONS] <torrent-file>")
	fmt.Println()
	fmt.Println("Options:")
	flag.PrintDefaults()
	fmt.Println()
	fmt.Println("Example:")
	fmt.Println("  torrent-client ubuntu.torrent")
	fmt.Println("  torrent-client -workers 50 -output ./downloads ubuntu.torrent")
}

// printTorrentInfo prints torrent metadata
func printTorrentInfo(meta *torrent.TorrentMeta, verbose bool) {
	fmt.Println()
	fmt.Println("Torrent Information:")
	fmt.Println("--------------------")
	fmt.Printf("  Name:         %s\n", meta.Name)
	fmt.Printf("  Size:         %.2f MB (%d bytes)\n",
		float64(meta.Length)/(1024*1024),
		meta.Length)
	fmt.Printf("  Pieces:       %d\n", meta.NumPieces())
	fmt.Printf("  Piece Size:   %.2f KB\n", float64(meta.PieceLength)/1024)
	fmt.Printf("  Info Hash:    %x\n", meta.InfoHash)

	if verbose {
		fmt.Println("\n  First 3 piece hashes:")
		for i := 0; i < min(3, len(meta.Pieces)); i++ {
			fmt.Printf("    [%d] %x\n", i, meta.Pieces[i])
		}
	}
}

// printPeerList prints a sample of peers
func printPeerList(peers []tracker.Peer, limit int) {
	fmt.Println("\n  Sample peers:")
	displayCount := min(limit, len(peers))
	for i := 0; i < displayCount; i++ {
		fmt.Printf("    [%d] %s\n", i+1, peers[i])
	}
	if len(peers) > limit {
		fmt.Printf("    ... and %d more\n", len(peers)-limit)
	}
}

