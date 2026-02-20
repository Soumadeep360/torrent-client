# How to Test the BitTorrent Client

Quick guide to test this project in 5 minutes or less.

## ⚡ Quick Start (3 steps)

### 1. Run Unit Tests

```bash
go test ./...
```

**Expected output**:
```
ok  	github.com/yourusername/torrent-client/internal/progress	0.463s
ok  	github.com/yourusername/torrent-client/internal/torrent	0.474s
```

✅ **All tests should PASS**

### 2. Test with Real Torrent (Recommended!)

```bash
# Download a small test torrent (~600MB, fast download)
wget https://cdimage.debian.org/debian-cd/current/amd64/bt-cd/debian-12.5.0-amd64-netinst.iso.torrent

# Build and run
go build -o torrent-client ./cmd/main.go
./torrent-client debian-12.5.0-amd64-netinst.iso.torrent
```

**Expected output**:
```
╔═══════════════════════════════════════╗
║     Go BitTorrent Client v1.0         ║
║   Demonstrating Go Concurrency        ║
╚═══════════════════════════════════════╝

=== Step 1: Parsing Torrent File ===
✓ Parsed successfully

=== Step 2: Contacting Tracker ===
✓ Found 127 peers

=== Starting Download ===
Progress: 10.0% (251/2516 pieces)
Progress: 50.0% (1258/2516 pieces)
Progress: 100.0% (2516/2516 pieces)

✓ Download complete!
```

### 3. Verify Download

```bash
# Check file was created
ls -lh debian-12.5.0-amd64-netinst.iso

# Should show ~629 MB file
-rw-r--r--  1 user  staff   629M Feb 19 20:30 debian-12.5.0-amd64-netinst.iso
```

---

## 🧪 More Testing Options

### Test Different Worker Counts

```bash
./torrent-client -workers 10 test.torrent   # Fewer workers
./torrent-client -workers 50 test.torrent   # More workers
```

### Test Verbose Mode

```bash
./torrent-client -verbose test.torrent
```

Shows detailed information including:
- All torrent metadata
- First 3 piece hashes
- Sample peer list

### Test Custom Output

```bash
mkdir downloads
./torrent-client -output ./downloads test.torrent
```

### Test Concurrency with Race Detector

```bash
go test ./... -race
```

Should show **no race conditions** ✅

### Run Benchmarks

```bash
go test ./internal/progress -bench=. -benchmem
```

Expected performance:
```
BenchmarkAddPiece-12          635587328    1.700 ns/op    0 B/op    0 allocs/op
BenchmarkGetStats-12           28497470   41.74 ns/op    0 B/op    0 allocs/op
```

AddPiece is **incredibly fast** (~1.7ns) thanks to atomic operations!

---

## 📝 What Gets Tested

### Unit Tests ✅
- **Torrent parser**: Piece splitting, size calculation
- **Progress tracker**: Concurrent updates, speed calculation
- **Atomic operations**: 10 goroutines updating simultaneously

### Concurrency ✅
- Worker pool pattern
- Channels for work distribution
- WaitGroup for synchronization
- Mutex for shared data
- Atomic counters (lock-free!)
- Concurrent file writes

### Real-World ✅
- Parse actual .torrent files
- Connect to real trackers
- Connect to real peers
- Download real files
- Verify SHA1 hashes

---

## 🎯 Success Checklist

After running tests, you should see:

- [x] ✅ Unit tests pass
- [x] ✅ No race conditions
- [x] ✅ Benchmarks show ns/op performance
- [x] ✅ Real torrent downloads
- [x] ✅ Progress updates in real-time
- [x] ✅ File created with correct size

---

## 💡 Recommended Test Torrents

### Small & Fast (~600MB)
```bash
wget https://cdimage.debian.org/debian-cd/current/amd64/bt-cd/debian-12.5.0-amd64-netinst.iso.torrent
```

### Medium (~4.5GB)
```bash
wget https://releases.ubuntu.com/22.04/ubuntu-22.04.3-desktop-amd64.iso.torrent
```

Both have **many active seeders** so download will be fast!

---

## 🐛 Troubleshooting

### Build fails
```bash
# Make sure you're in the project directory
cd torrent-client

# Clean and rebuild
go clean
go build -o torrent-client ./cmd/main.go
```

### Tests fail
```bash
# Run with verbose to see what's wrong
go test ./... -v
```

### "No peers available"
- Torrent might be old/dead
- Use Debian or Ubuntu torrents (always have seeders)

### Download is slow
```bash
# Try with more workers
./torrent-client -workers 50 test.torrent
```

---

## 🚀 One-Command Full Test

```bash
# This tests everything!
go test ./... && \
go test ./... -race && \
go build -o torrent-client ./cmd/main.go && \
echo "✅ All tests passed! Now test with a real torrent:"
echo "./torrent-client your-file.torrent"
```

---

## 📊 What You'll Learn

By testing this project, you'll see:

1. **Go unit tests** in action
2. **Concurrent code testing** with race detector
3. **Benchmarking** atomic operations
4. **Real-world BitTorrent** protocol
5. **Worker pool pattern** in practice
6. **Progress tracking** with atomic counters

---

## ⏱️ Time Estimates

- **Unit tests**: 1 second
- **Race detector**: 3-5 seconds
- **Benchmarks**: 5 seconds
- **Small torrent download**: 2-10 minutes (depends on internet)
- **Large torrent download**: 10-30 minutes (depends on internet)

---

## 🎉 That's It!

**Simplest test**:
```bash
go test ./... && echo "✅ Tests pass!"
```

**Best test**:
```bash
wget https://cdimage.debian.org/debian-cd/current/amd64/bt-cd/debian-12.5.0-amd64-netinst.iso.torrent
./torrent-client debian-12.5.0-amd64-netinst.iso.torrent
# Watch it download! 🎊
```

For complete testing details, see [TESTING_GUIDE.md](TESTING_GUIDE.md).
