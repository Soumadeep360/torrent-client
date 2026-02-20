# Go BitTorrent Client

A production-quality BitTorrent client implementation in Go, designed to demonstrate **Go concurrency patterns** and **distributed systems concepts**. This project is perfect for learning, interviews, and understanding how BitTorrent works under the hood.

## 🎯 Project Goals

- **Learn Go Concurrency**: Practical implementation of goroutines, channels, worker pools, and synchronization primitives
- **Understand BitTorrent**: Complete protocol implementation from parsing to downloading
- **Clean Architecture**: Modular, testable, and interview-ready code structure
- **Production Quality**: Error handling, logging, and real-world considerations

## ✨ Features

✅ Parse .torrent files (bencode format)
✅ Connect to HTTP trackers
✅ Discover and connect to peers
✅ Concurrent piece downloading with worker pool
✅ Real-time progress tracking
✅ Concurrent file writing using `WriteAt`
✅ SHA1 hash verification
✅ Configurable worker pool size
✅ Clean, modular architecture

## 🏗️ Architecture

```
torrent-client/
├── cmd/
│   └── main.go                 # CLI entry point
│
├── internal/
│   ├── torrent/
│   │   └── parser.go           # Torrent file parsing (bencode)
│   │
│   ├── tracker/
│   │   └── tracker.go          # HTTP tracker communication
│   │
│   ├── peer/
│   │   └── peer.go             # Peer connection & protocol
│   │
│   ├── downloader/
│   │   ├── manager.go          # Download coordinator
│   │   ├── worker.go           # Worker pool implementation
│   │   └── orchestrator.go    # High-level orchestration
│   │
│   ├── filewriter/
│   │   └── writer.go           # Concurrent file writer
│   │
│   └── progress/
│       └── progress.go         # Progress tracking
│
├── go.mod
└── README.md
```

## 🔧 How It Works

### 1. **Torrent File Parsing**
The client reads a `.torrent` file and extracts:
- Tracker URL (announce)
- File name and size
- Piece length and piece hashes (SHA1)
- Info hash (unique identifier)

### 2. **Tracker Communication**
Contacts the tracker via HTTP GET request with:
- Info hash
- Peer ID (randomly generated)
- Port, uploaded, downloaded, left
- Requests compact peer format (6 bytes per peer)

### 3. **Peer Discovery & Connection**
- Receives list of peers from tracker (IP:Port)
- Establishes TCP connections to peers
- Performs BitTorrent handshake
- Verifies info hash matches

### 4. **Concurrent Download (Worker Pool Pattern)**

This is the **core concurrency demonstration**:

```go
// Work queue: channel of pieces to download
workQueue := make(chan PieceWork)

// Spawn worker goroutines
for i := 0; i < numWorkers; i++ {
    go worker(workQueue, resultQueue)
}

// Workers pull from queue concurrently
for work := range workQueue {
    // Download piece from peer
    // Send result to result queue
}
```

**Concurrency primitives used:**
- **Goroutines**: Each worker runs in its own goroutine
- **Channels**: Work distribution and result collection
- **sync.WaitGroup**: Coordinating worker completion
- **sync.Mutex**: Protecting shared data
- **sync/atomic**: Lock-free counters for progress

### 5. **Piece Download Process**

Each worker:
1. Takes a piece from the work queue
2. Connects to a peer
3. Sends "interested" message
4. Waits for "unchoke"
5. Requests piece in 16KB blocks
6. Assembles blocks into complete piece
7. Verifies SHA1 hash
8. Sends result to result queue

### 6. **File Writing**

Uses `os.File.WriteAt()` for **thread-safe concurrent writes**:
```go
// WriteAt writes to specific offset without locking file position
// Safe to call from multiple goroutines simultaneously
file.WriteAt(pieceData, offset)
```

### 7. **Progress Tracking**

Uses **atomic operations** for lock-free updates:
```go
atomic.AddInt32(&downloadedPieces, 1)
atomic.AddInt64(&downloadedBytes, pieceSize)
```

## 🚀 Usage

### Build
```bash
go build -o torrent-client ./cmd/main.go
```

### Run
```bash
# Basic usage
./torrent-client ubuntu.torrent

# With options
./torrent-client -workers 50 -output ./downloads ubuntu.torrent

# Verbose mode
./torrent-client -verbose ubuntu.torrent
```

### Options
- `-workers <n>`: Number of concurrent workers (default: 20)
- `-output <dir>`: Output directory (default: current directory)
- `-verbose`: Enable verbose output

## 🧪 Testing with Public Torrents

You can test with any legal public torrent. Popular options:

### Ubuntu ISO (Recommended for testing)
```bash
# Download Ubuntu torrent file
wget https://releases.ubuntu.com/22.04/ubuntu-22.04.3-desktop-amd64.iso.torrent

# Run client
./torrent-client ubuntu-22.04.3-desktop-amd64.iso.torrent
```

### Other Legal Torrents
- Linux distributions (Debian, Fedora, etc.)
- Open source software
- Public domain media

## 📊 Sample Output

```
╔═══════════════════════════════════════╗
║     Go BitTorrent Client v1.0         ║
║   Demonstrating Go Concurrency        ║
╚═══════════════════════════════════════╝

=== Step 1: Parsing Torrent File ===
Reading: ubuntu.torrent

Torrent Information:
--------------------
  Name:         ubuntu-22.04.3-desktop-amd64.iso
  Size:         4692.00 MB (4920000000 bytes)
  Pieces:       1175
  Piece Size:   4096.00 KB
  Info Hash:    a1b2c3d4e5f6...

✓ Parsed successfully

=== Step 2: Contacting Tracker ===
Tracker: http://torrent.ubuntu.com:6969/announce
✓ Found 127 peers

=== Step 3: Preparing Download ===
Workers: 20
Output: ubuntu-22.04.3-desktop-amd64.iso

=== Starting Download ===
File: ubuntu-22.04.3-desktop-amd64.iso
Size: 4692.00 MB (4920000000 bytes)
Pieces: 1175
Workers: 20
Peers: 127

Downloading...
Progress: 25.3% (297/1175 pieces)
Progress: 50.7% (596/1175 pieces)
Progress: 75.1% (883/1175 pieces)
Progress: 100.0% (1175/1175 pieces)

✓ Download complete!
File saved to: ubuntu-22.04.3-desktop-amd64.iso
```

## 🎓 Key Concepts Demonstrated

### 1. **Goroutines**
```go
// Spawning concurrent workers
for i := 0; i < numWorkers; i++ {
    go worker(workQueue, resultQueue)
}
```

### 2. **Channels**
```go
// Work distribution channel
workQueue := make(chan PieceWork, totalPieces)

// Result collection channel
resultQueue := make(chan PieceResult)
```

### 3. **Worker Pool Pattern**
```go
// Classic worker pool pattern
for work := range workQueue {
    result := processWork(work)
    resultQueue <- result
}
```

### 4. **Synchronization with WaitGroup**
```go
var wg sync.WaitGroup
for i := 0; i < numWorkers; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        worker()
    }()
}
wg.Wait()
```

### 5. **Mutex for Shared Data**
```go
mu.Lock()
pieces[index].Data = downloadedData
mu.Unlock()
```

### 6. **Atomic Operations**
```go
atomic.AddInt32(&downloadedCount, 1)
count := atomic.LoadInt32(&downloadedCount)
```

### 7. **Context for Cancellation** (extensible)
```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
```

## 🧩 BitTorrent Protocol Overview

### Handshake
```
<pstrlen><pstr><reserved><info_hash><peer_id>
  1 byte | 19  |   8     |    20    |   20
```

### Messages
- **Choke (0)**: Peer is not sending pieces
- **Unchoke (1)**: Peer will send pieces
- **Interested (2)**: We want pieces
- **Have (4)**: Peer has a piece
- **Bitfield (5)**: Which pieces peer has
- **Request (6)**: Request a block
- **Piece (7)**: Block data

### Request Message
```
<index><begin><length>
  4B     4B      4B
```

## 💡 Interview Talking Points

### Concurrency Design
- **Why worker pool?** Limits resource usage, prevents connection storms
- **Why channels?** Type-safe, prevents race conditions, clean communication
- **Why atomic?** Lock-free performance for simple counters

### Performance Considerations
- **Piece size**: Standard 256KB-4MB pieces
- **Block size**: 16KB blocks for pipelining
- **Worker count**: Balance between throughput and connection overhead
- **WriteAt**: No file seeking overhead, true concurrent writes

### Error Handling
- **Connection failures**: Retry with different peer
- **Hash mismatches**: Request piece again
- **Tracker failures**: Could implement multi-tracker support

### Scalability
- **100s of peers**: Worker pool prevents overwhelming system
- **Large files**: WriteAt allows sparse file writing
- **Progress tracking**: Atomic operations scale to high update rates

## 🔮 Future Enhancements

- [ ] UDP tracker support
- [ ] DHT (Distributed Hash Table) for trackerless operation
- [ ] Upload/seeding capability
- [ ] Magnet link support
- [ ] Piece priority (sequential/rarest-first)
- [ ] Web UI with WebSocket progress
- [ ] Resume capability
- [ ] Rate limiting
- [ ] Multiple file torrents

## 📚 Dependencies

```go
require github.com/jackpal/bencode-go v1.0.2
```

Minimal dependencies - most functionality is pure Go stdlib.

## 🎯 Learning Outcomes

After studying this project, you will understand:

✅ Go concurrency patterns (goroutines, channels, select)
✅ Synchronization primitives (Mutex, WaitGroup, atomic)
✅ Worker pool pattern
✅ Network programming (TCP, HTTP)
✅ Binary protocol implementation
✅ Concurrent I/O operations
✅ Clean architecture and modularity
✅ Error handling in distributed systems
✅ BitTorrent protocol internals

## 📖 References

- [BitTorrent Protocol Specification (BEP 3)](http://www.bittorrent.org/beps/bep_0003.html)
- [Kristen Widman's BitTorrent Guide](https://blog.jse.li/posts/torrent/)
- [Go Concurrency Patterns](https://go.dev/blog/pipelines)
- [Bencode Specification](https://wiki.theory.org/BitTorrentSpecification#Bencoding)

## ⚖️ License

This project is for educational purposes. Respect copyright laws when downloading content.

## 🙏 Acknowledgments

Built as a learning project to understand:
- Go's concurrency model
- Distributed systems design
- BitTorrent protocol implementation

Perfect for interview preparation and demonstrating production Go code.

---

**Built with Go 1.21+** | [Report Issues](https://github.com/yourusername/torrent-client/issues)
