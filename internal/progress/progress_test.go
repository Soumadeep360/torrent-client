package progress

import (
	"sync"
	"testing"
	"time"
)

func TestNewTracker(t *testing.T) {
	tracker := NewTracker(100, 10000)

	if tracker.totalPieces != 100 {
		t.Errorf("totalPieces = %d, want 100", tracker.totalPieces)
	}

	if tracker.totalBytes != 10000 {
		t.Errorf("totalBytes = %d, want 10000", tracker.totalBytes)
	}
}

func TestAddPiece(t *testing.T) {
	tracker := NewTracker(10, 1000)

	// Add a piece
	tracker.AddPiece(100)

	stats := tracker.GetStats()
	if stats.DownloadedPieces != 1 {
		t.Errorf("DownloadedPieces = %d, want 1", stats.DownloadedPieces)
	}

	if stats.DownloadedBytes != 100 {
		t.Errorf("DownloadedBytes = %d, want 100", stats.DownloadedBytes)
	}

	// Add another piece
	tracker.AddPiece(200)

	stats = tracker.GetStats()
	if stats.DownloadedPieces != 2 {
		t.Errorf("DownloadedPieces = %d, want 2", stats.DownloadedPieces)
	}

	if stats.DownloadedBytes != 300 {
		t.Errorf("DownloadedBytes = %d, want 300", stats.DownloadedBytes)
	}
}

func TestConcurrentAddPiece(t *testing.T) {
	tracker := NewTracker(1000, 100000)
	numGoroutines := 10
	piecesPerGoroutine := 100

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < piecesPerGoroutine; j++ {
				tracker.AddPiece(100)
			}
		}()
	}

	wg.Wait()

	stats := tracker.GetStats()
	expectedPieces := numGoroutines * piecesPerGoroutine
	expectedBytes := int64(expectedPieces * 100)

	if stats.DownloadedPieces != expectedPieces {
		t.Errorf("DownloadedPieces = %d, want %d", stats.DownloadedPieces, expectedPieces)
	}

	if stats.DownloadedBytes != expectedBytes {
		t.Errorf("DownloadedBytes = %d, want %d", stats.DownloadedBytes, expectedBytes)
	}
}

func TestGetPercentComplete(t *testing.T) {
	tracker := NewTracker(100, 10000)

	// 0%
	if pct := tracker.GetPercentComplete(); pct != 0 {
		t.Errorf("GetPercentComplete() = %f, want 0", pct)
	}

	// 50%
	tracker.AddPiece(5000)
	pct := tracker.GetPercentComplete()
	if pct < 49.9 || pct > 50.1 {
		t.Errorf("GetPercentComplete() = %f, want ~50", pct)
	}

	// 100%
	tracker.AddPiece(5000)
	pct = tracker.GetPercentComplete()
	if pct < 99.9 || pct > 100.1 {
		t.Errorf("GetPercentComplete() = %f, want ~100", pct)
	}
}

func TestIsComplete(t *testing.T) {
	tracker := NewTracker(3, 300)

	if tracker.IsComplete() {
		t.Error("IsComplete() = true, want false (no pieces downloaded)")
	}

	tracker.AddPiece(100)
	tracker.AddPiece(100)

	if tracker.IsComplete() {
		t.Error("IsComplete() = true, want false (2/3 pieces)")
	}

	tracker.AddPiece(100)

	if !tracker.IsComplete() {
		t.Error("IsComplete() = false, want true (3/3 pieces)")
	}
}

func TestDownloadSpeed(t *testing.T) {
	tracker := NewTracker(100, 10000)

	// Wait a bit to ensure time passes
	time.Sleep(100 * time.Millisecond)

	// Add some data
	tracker.AddPiece(1000)

	stats := tracker.GetStats()

	// Speed should be positive
	if stats.DownloadSpeed <= 0 {
		t.Errorf("DownloadSpeed = %f, want > 0", stats.DownloadSpeed)
	}

	// For 1000 bytes in ~100ms, speed should be around 10000 bytes/sec
	// But give it a wide range due to timing variations
	if stats.DownloadSpeed < 1000 || stats.DownloadSpeed > 100000 {
		t.Logf("Warning: DownloadSpeed = %f seems unusual", stats.DownloadSpeed)
	}
}

func TestReset(t *testing.T) {
	tracker := NewTracker(100, 10000)

	// Add some data
	tracker.AddPiece(500)
	tracker.AddPiece(500)

	// Verify data was added
	stats := tracker.GetStats()
	if stats.DownloadedPieces != 2 {
		t.Fatalf("Setup failed: DownloadedPieces = %d, want 2", stats.DownloadedPieces)
	}

	// Reset
	tracker.Reset()

	// Verify reset
	stats = tracker.GetStats()
	if stats.DownloadedPieces != 0 {
		t.Errorf("After reset: DownloadedPieces = %d, want 0", stats.DownloadedPieces)
	}

	if stats.DownloadedBytes != 0 {
		t.Errorf("After reset: DownloadedBytes = %d, want 0", stats.DownloadedBytes)
	}
}

func BenchmarkAddPiece(b *testing.B) {
	tracker := NewTracker(1000000, 100000000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker.AddPiece(100)
	}
}

func BenchmarkGetStats(b *testing.B) {
	tracker := NewTracker(1000, 100000)
	tracker.AddPiece(5000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tracker.GetStats()
	}
}

func BenchmarkConcurrentAddPiece(b *testing.B) {
	tracker := NewTracker(1000000, 100000000)
	numGoroutines := 10

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		for j := 0; j < numGoroutines; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				tracker.AddPiece(100)
			}()
		}
		wg.Wait()
	}
}
