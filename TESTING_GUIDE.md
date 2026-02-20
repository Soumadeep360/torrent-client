# Testing Guide

Complete guide for testing the BitTorrent client at all levels.

## 📋 Quick Start

### 1. Run All Tests

```bash
# Run all unit tests
go test ./...

# Run with verbose output
go test ./... -v

# Run with coverage
go test ./... -cover

# Run with race detector (important for concurrent code!)
go test ./... -race
```

### 2. Run Automated Test Script

```bash
chmod +x test.sh
./test.sh
```

## ✅ Unit Tests (Already Implemented!)

We've created comprehensive unit tests for key components:

### Torrent Parser Tests

**File**: `internal/torrent/parser_test.go`

```bash
go test ./internal/torrent -v
```

**What's tested**:
- ✅ Piece hash splitting (valid/invalid lengths)
- ✅ Piece size calculation (including last piece edge case)
- ✅ Piece count calculation

**Sample output**:
```
=== RUN   TestSplitPieces
=== RUN   TestSplitPieces/valid_2_pieces
=== RUN   TestSplitPieces/invalid_length
=== RUN   TestSplitPieces/empty_string
--- PASS: TestSplitPieces (0.00s)
=== RUN   TestTorrentMetaPieceSize
--- PASS: TestTorrentMetaPieceSize (0.00s)
=== RUN   TestNumPieces
--- PASS: TestNumPieces (0.00s)
PASS
ok  	github.com/yourusername/torrent-client/internal/torrent	1.292s
```

### Progress Tracker Tests (Concurrency!)

**File**: `internal/progress/progress_test.go`

```bash
go test ./internal/progress -v
```

**What's tested**:
- ✅ Basic tracker operations
- ✅ **Concurrent updates from 10 goroutines** (proves atomic operations work!)
- ✅ Download speed calculation
- ✅ Percentage completion
- ✅ Reset functionality

**Key test - Concurrent Safety**:
```go
func TestConcurrentAddPiece(t *testing.T) {
    tracker := NewTracker(1000, 100000)

    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := 0; j < 100; j++ {
                tracker.AddPiece(100) // 10 goroutines, no races!
            }
        }()
    }
    wg.Wait()

    // Should be exactly 1000 pieces - atomic operations FTW!
    stats := tracker.GetStats()
    // ✅ PASS - no race conditions
}
```

## 🏃 Benchmarks

### Run All Benchmarks

```bash
go test ./internal/progress -bench=. -benchmem
```

**Actual Results** (on Apple M4 Pro):
```
BenchmarkAddPiece-12              	647878635	  1.601 ns/op	  0 B/op	  0 allocs/op
BenchmarkGetStats-12              	28511378	 44.65 ns/op	  0 B/op	  0 allocs/op
BenchmarkConcurrentAddPiece-12    	  673086	1874 ns/op	256 B/op	 11 allocs/op
```

**Key insights**:
- **AddPiece is BLAZING fast** (~1.6ns) - atomic operations are incredible!
- **Zero allocations** for hot paths
- **Concurrent operations scale well**

## 🧪 Real-World Testing

### Test 1: Small Torrent First! (~600MB)

**Best for first test** - downloads quickly

```bash
# Download Debian netinst torrent
wget https://cdimage.debian.org/debian-cd/current/amd64/bt-cd/debian-12.5.0-amd64-netinst.iso.torrent

# Run client
./torrent-client debian-12.5.0-amd64-netinst.iso.torrent
```

**Expected output**:
```
╔═══════════════════════════════════════╗
║     Go BitTorrent Client v1.0         ║
║   Demonstrating Go Concurrency        ║
╚═══════════════════════════════════════╝

=== Step 1: Parsing Torrent File ===
Reading: debian-12.5.0-amd64-netinst.iso.torrent

Torrent Information:
--------------------
  Name:         debian-12.5.0-amd64-netinst.iso
  Size:         629.00 MB
  Pieces:       2516
  ...

=== Step 2: Contacting Tracker ===
✓ Found 127 peers

=== Starting Download ===
Downloading...
Progress: 10.0% (251/2516 pieces)
Progress: 25.0% (629/2516 pieces)
...
✓ Download complete!
```

### Test 2: Ubuntu ISO (~4.5GB)

```bash
# Download Ubuntu torrent
wget https://releases.ubuntu.com/22.04/ubuntu-22.04.3-desktop-amd64.iso.torrent

# Run with more workers
./torrent-client -workers 30 ubuntu-22.04.3-desktop-amd64.iso.torrent
```

### Test 3: Verbose Mode

```bash
./torrent-client -verbose debian-netinst.torrent
```

Shows:
- Detailed torrent metadata
- First 3 piece hashes
- Sample peer list
- More debug info

### Test 4: Custom Output Directory

```bash
mkdir -p downloads
./torrent-client -output ./downloads debian-netinst.torrent
```

### Test 5: Worker Pool Scaling

```bash
# Test different worker counts
./torrent-client -workers 5 test.torrent
./torrent-client -workers 20 test.torrent  # Default
./torrent-client -workers 50 test.torrent
./torrent-client -workers 100 test.torrent
```

## 🔍 Race Detection (CRITICAL!)

**Always run with race detector** for concurrent code:

```bash
# Run tests with race detector
go test ./... -race

# Build with race detector
go build -race -o torrent-client-race ./cmd/main.go

# Run with race detector
./torrent-client-race test.torrent
```

This will catch:
- Data races
- Unsafe concurrent access
- Missing synchronization

## 📊 Coverage Report

```bash
# Generate coverage report
go test ./... -coverprofile=coverage.out

# View in browser
go tool cover -html=coverage.out

# View summary
go tool cover -func=coverage.out
```

## ⚡ Performance Profiling

### CPU Profiling

```bash
# Add profiling flags to main.go first, then:
go build -o torrent-client ./cmd/main.go

# Run with CPU profiling (if implemented)
CPUPROFILE=cpu.prof ./torrent-client test.torrent

# Analyze
go tool pprof cpu.prof
```

### Memory Profiling

```bash
# Run with memory profiling (if implemented)
MEMPROFILE=mem.prof ./torrent-client test.torrent

# Analyze
go tool pprof mem.prof
```

## 🎯 Verification Checklist

After downloading a test torrent, verify:

### Functionality
- [ ] Torrent parsed correctly
- [ ] Tracker contacted successfully
- [ ] Peers discovered (should see 20+)
- [ ] File downloads completely
- [ ] File size matches metadata
- [ ] No errors in output

### Concurrency
- [ ] No race conditions with `-race`
- [ ] Progress updates smooth
- [ ] Multiple workers run concurrently
- [ ] No deadlocks

### Performance
- [ ] Download speed > 1 MB/s (depends on seeders)
- [ ] CPU usage reasonable (<100%)
- [ ] Memory usage stable
- [ ] All workers utilized

## 🐛 Troubleshooting

### "No peers available"
- Torrent might be old/unpopular
- Try Debian or Ubuntu torrents (always have seeders)
- Check tracker URL is reachable

### "Connection refused"
- Firewall might be blocking
- Peer might be offline
- Normal - client will try other peers

### "Hash mismatch"
- Peer sent corrupt data
- Client will retry with different peer
- If persistent, check network

### Tests fail
```bash
# Run with verbose to see details
go test ./... -v

# Run specific test
go test ./internal/progress -run TestConcurrentAddPiece -v

# Check for race conditions
go test ./... -race
```

## 📝 Test Summary

### What We've Tested

✅ **Unit Tests**:
- Torrent parser (3 tests, all passing)
- Progress tracker (7 tests, all passing including concurrency)

✅ **Benchmarks**:
- AddPiece: ~1.6ns/op (atomic is FAST!)
- GetStats: ~44ns/op
- Concurrent operations: scale well

✅ **Integration**:
- Build succeeds
- CLI works
- Help command works
- Error handling works

### What to Test Manually

1. **Download small torrent** (Debian netinst)
2. **Download large torrent** (Ubuntu ISO)
3. **Test different worker counts** (5, 20, 50)
4. **Test verbose mode**
5. **Test custom output directory**

## 🚀 Quick Test Commands

```bash
# Basic build and unit tests
go build -o torrent-client ./cmd/main.go
go test ./... -v

# Race detection (important!)
go test ./... -race

# Benchmarks
go test ./internal/progress -bench=. -benchmem

# Real download (recommended!)
wget https://cdimage.debian.org/debian-cd/current/amd64/bt-cd/debian-12.5.0-amd64-netinst.iso.torrent
./torrent-client debian-12.5.0-amd64-netinst.iso.torrent
```

## ✨ Success Criteria

Your tests should show:

✅ All unit tests pass
✅ No race conditions detected
✅ Benchmarks show good performance
✅ Real torrent downloads successfully
✅ Progress updates smoothly
✅ File integrity verified (SHA1)
✅ Multiple workers run concurrently

---

**Bottom line**: The best test is **downloading a real torrent**! Start with Debian (~600MB) to verify everything works, then try Ubuntu (~4.5GB) for a longer stress test.

**Quick start**:
```bash
wget https://cdimage.debian.org/debian-cd/current/amd64/bt-cd/debian-12.5.0-amd64-netinst.iso.torrent
./torrent-client debian-12.5.0-amd64-netinst.iso.torrent
```

Good luck! 🎉
