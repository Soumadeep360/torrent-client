package progress

import (
	"fmt"
	"sync/atomic"
	"time"
)

// Tracker tracks download progress in a thread-safe manner
type Tracker struct {
	totalPieces     int32   // Total number of pieces
	totalBytes      int64   // Total file size in bytes
	downloadedPieces int32  // Number of pieces downloaded (atomic)
	downloadedBytes  int64  // Number of bytes downloaded (atomic)
	startTime       time.Time
	lastUpdate      time.Time
	lastBytes       int64
}

// Stats represents download statistics at a point in time
type Stats struct {
	TotalPieces      int
	DownloadedPieces int
	TotalBytes       int64
	DownloadedBytes  int64
	PercentComplete  float64
	DownloadSpeed    float64 // Bytes per second
	TimeElapsed      time.Duration
	EstimatedTimeRemaining time.Duration
}

// NewTracker creates a new progress tracker
func NewTracker(totalPieces int, totalBytes int64) *Tracker {
	now := time.Now()
	return &Tracker{
		totalPieces: int32(totalPieces),
		totalBytes:  totalBytes,
		startTime:   now,
		lastUpdate:  now,
	}
}

// AddPiece increments the downloaded piece count
// This method is thread-safe and can be called from multiple goroutines
func (t *Tracker) AddPiece(pieceSize int) {
	atomic.AddInt32(&t.downloadedPieces, 1)
	atomic.AddInt64(&t.downloadedBytes, int64(pieceSize))
}

// GetStats returns current download statistics
func (t *Tracker) GetStats() Stats {
	downloaded := atomic.LoadInt32(&t.downloadedPieces)
	downloadedBytes := atomic.LoadInt64(&t.downloadedBytes)

	totalPieces := int(atomic.LoadInt32(&t.totalPieces))
	totalBytes := atomic.LoadInt64(&t.totalBytes)

	// Calculate percentage
	var percent float64
	if totalBytes > 0 {
		percent = float64(downloadedBytes) / float64(totalBytes) * 100
	}

	// Calculate download speed
	now := time.Now()
	elapsed := now.Sub(t.startTime)
	var speed float64
	if elapsed.Seconds() > 0 {
		speed = float64(downloadedBytes) / elapsed.Seconds()
	}

	// Estimate time remaining
	var eta time.Duration
	if speed > 0 && totalBytes > downloadedBytes {
		remainingBytes := totalBytes - downloadedBytes
		remainingSeconds := float64(remainingBytes) / speed
		eta = time.Duration(remainingSeconds) * time.Second
	}

	return Stats{
		TotalPieces:            totalPieces,
		DownloadedPieces:       int(downloaded),
		TotalBytes:             totalBytes,
		DownloadedBytes:        downloadedBytes,
		PercentComplete:        percent,
		DownloadSpeed:          speed,
		TimeElapsed:            elapsed,
		EstimatedTimeRemaining: eta,
	}
}

// GetPercentComplete returns the download completion percentage
func (t *Tracker) GetPercentComplete() float64 {
	downloadedBytes := atomic.LoadInt64(&t.downloadedBytes)
	totalBytes := atomic.LoadInt64(&t.totalBytes)

	if totalBytes == 0 {
		return 0
	}

	return float64(downloadedBytes) / float64(totalBytes) * 100
}

// IsComplete returns true if download is complete
func (t *Tracker) IsComplete() bool {
	downloaded := atomic.LoadInt32(&t.downloadedPieces)
	total := atomic.LoadInt32(&t.totalPieces)
	return downloaded >= total
}

// FormatStats returns a formatted string of current statistics
func (s Stats) String() string {
	// Format bytes
	downloadedMB := float64(s.DownloadedBytes) / (1024 * 1024)
	totalMB := float64(s.TotalBytes) / (1024 * 1024)
	speedMBps := s.DownloadSpeed / (1024 * 1024)

	// Format time
	elapsed := s.TimeElapsed.Round(time.Second)
	eta := s.EstimatedTimeRemaining.Round(time.Second)

	return fmt.Sprintf(
		"Progress: %.1f%% (%.2f MB / %.2f MB) | "+
		"Pieces: %d/%d | "+
		"Speed: %.2f MB/s | "+
		"Elapsed: %s | "+
		"ETA: %s",
		s.PercentComplete,
		downloadedMB,
		totalMB,
		s.DownloadedPieces,
		s.TotalPieces,
		speedMBps,
		elapsed,
		eta,
	)
}

// FormatSimple returns a simple progress string
func (s Stats) FormatSimple() string {
	return fmt.Sprintf("Progress: %.1f%% (%d/%d pieces)",
		s.PercentComplete,
		s.DownloadedPieces,
		s.TotalPieces,
	)
}

// FormatSpeed returns just the download speed
func (s Stats) FormatSpeed() string {
	speedMBps := s.DownloadSpeed / (1024 * 1024)
	speedKBps := s.DownloadSpeed / 1024

	if speedMBps >= 1.0 {
		return fmt.Sprintf("%.2f MB/s", speedMBps)
	}
	return fmt.Sprintf("%.2f KB/s", speedKBps)
}

// DisplayLoop continuously displays progress updates
// This should be run in a goroutine
func (t *Tracker) DisplayLoop(interval time.Duration, stopChan <-chan bool) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			stats := t.GetStats()
			fmt.Printf("\r%s", stats.FormatSimple())

			if stats.PercentComplete >= 100 {
				fmt.Println() // New line when complete
				return
			}
		case <-stopChan:
			fmt.Println() // New line before stopping
			return
		}
	}
}

// PrintStats prints detailed statistics
func (t *Tracker) PrintStats() {
	stats := t.GetStats()
	fmt.Println(stats.String())
}

// Reset resets the progress tracker
func (t *Tracker) Reset() {
	atomic.StoreInt32(&t.downloadedPieces, 0)
	atomic.StoreInt64(&t.downloadedBytes, 0)
	now := time.Now()
	t.startTime = now
	t.lastUpdate = now
	t.lastBytes = 0
}
