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
# (Debian "current" version changes; check https://cdimage.debian.org/debian-cd/current/amd64/bt-cd/ if 404)
wget https://cdimage.debian.org/debian-cd/current/amd64/bt-cd/debian-13.3.0-amd64-netinst.iso.torrent
# On Windows PowerShell, use instead (saves file and avoids security prompt):
# Invoke-WebRequest -Uri "https://cdimage.debian.org/debian-cd/current/amd64/bt-cd/debian-13.3.0-amd64-netinst.iso.torrent" -OutFile "debian-13.3.0-amd64-netinst.iso.torrent" -UseBasicParsing

# Build and run
go build -o torrent-client ./cmd/main.go
./torrent-client debian-13.3.0-amd64-netinst.iso.torrent
# On Windows: .\torrent-client.exe debian-13.3.0-amd64-netinst.iso.torrent
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

The client already verifies integrity automatically:

- **Per piece**: Each piece is checked against its SHA1 hash from the torrent before being written. Bad data is rejected and re-downloaded.
- **After completion**: Total file size is checked; if it doesn’t match the torrent, the run fails with "file size verification failed".

So if you see **"✓ Download complete!"** and no error, the file matches the torrent (correct size and piece hashes).

**Quick check (file exists and size):**

```bash
# Linux/macOS
ls -lh debian-13.3.0-amd64-netinst.iso
# Should show ~754 MB for 13.3.0 netinst

# Windows PowerShell
Get-Item .\debian-13.3.0-amd64-netinst.iso | Select-Object Name, Length
```

**Full verification with official checksums (recommended for ISOs):**

Debian publishes SHA256 checksums. Use them to confirm the file matches the official image.

1. Download the checksum file (same directory as the torrent on the mirror):

   ```bash
   # Example: same base URL as the .torrent
   # https://cdimage.debian.org/debian-cd/current/amd64/bt-cd/ → ../iso-cd/ has SHA256SUMS
   wget https://cdimage.debian.org/debian-cd/current/amd64/iso-cd/SHA256SUMS
   # Windows PowerShell:
   # Invoke-WebRequest -Uri "https://cdimage.debian.org/debian-cd/current/amd64/iso-cd/SHA256SUMS" -OutFile "SHA256SUMS" -UseBasicParsing
   ```

2. Verify the ISO:

   **Linux/macOS:**
   ```bash
   sha256sum -c --ignore-missing SHA256SUMS
   # Or: grep "debian-13.3.0-amd64-netinst.iso" SHA256SUMS | sha256sum -c -
   ```

   **Windows PowerShell:**
   ```powershell
   Get-FileHash -Algorithm SHA256 .\debian-13.3.0-amd64-netinst.iso
   # Compare the displayed hash with the line for that filename in SHA256SUMS
   ```

If the hash matches the line in `SHA256SUMS`, the download is complete and matches the official image. For full authenticity you can also verify the GPG signature of `SHA256SUMS`; see [Debian CD verify](https://www.debian.org/CD/verify).

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
# Current version (check cdimage.debian.org if 404)
wget https://cdimage.debian.org/debian-cd/current/amd64/bt-cd/debian-13.3.0-amd64-netinst.iso.torrent
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
wget https://cdimage.debian.org/debian-cd/current/amd64/bt-cd/debian-13.3.0-amd64-netinst.iso.torrent
./torrent-client debian-13.3.0-amd64-netinst.iso.torrent
# Watch it download! 🎊
```

For complete testing details, see [TESTING_GUIDE.md](TESTING_GUIDE.md).
