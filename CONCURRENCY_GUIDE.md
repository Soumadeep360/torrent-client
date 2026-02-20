# Go Concurrency Patterns in This Project

This document highlights all the concurrency patterns and primitives used in the BitTorrent client, organized by location and purpose.

## 📋 Table of Contents

1. [Goroutines](#goroutines)
2. [Channels](#channels)
3. [Worker Pool Pattern](#worker-pool-pattern)
4. [sync.WaitGroup](#syncwaitgroup)
5. [sync.Mutex](#syncmutex)
6. [sync/atomic](#syncatomic)
7. [Concurrent File I/O](#concurrent-file-io)

---

## 1. Goroutines

### Location: `internal/downloader/manager.go`

```go
// Spawning worker goroutines in the worker pool
for i := 0; i < m.numWorkers; i++ {
    wg.Add(1)
    go func(workerID int) {
        defer wg.Done()
        m.runWorker(workerID)
    }(i)
}
```

**Purpose**: Each worker runs concurrently to download pieces in parallel.

**Interview Point**: "We spawn N worker goroutines that process pieces concurrently. Each goroutine is lightweight (~2KB stack) so we can easily run 20-50 workers."

---

### Location: `cmd/main.go` (Phase 3 test)

```go
// Concurrent peer connection attempts
for i := 0; i < connectLimit; i++ {
    wg.Add(1)
    go func(p tracker.Peer, index int) {
        defer wg.Done()
        // Attempt connection
    }(peers[i], i)
}
```

**Purpose**: Connect to multiple peers simultaneously to quickly find responsive ones.

---

## 2. Channels

### Location: `internal/downloader/manager.go`

```go
// Work distribution channel (buffered)
workQueue chan PieceWork

// Result collection channel (unbuffered)
resultQueue chan PieceResult

// Filling the work queue
for i := 0; i < len(m.pieces); i++ {
    m.workQueue <- PieceWork{...}
}
close(m.workQueue) // Signal no more work
```

**Purpose**:
- `workQueue`: Distributes pieces to workers (buffered for performance)
- `resultQueue`: Collects downloaded pieces (unbuffered for back-pressure)

**Interview Point**: "Channels provide type-safe, thread-safe communication. Buffered workQueue prevents blocking when adding work. Unbuffered resultQueue provides natural back-pressure."

---

## 3. Worker Pool Pattern

### Location: `internal/downloader/worker.go`

```go
// Classic worker pool pattern
func (m *Manager) runWorker(workerID int) {
    // Each worker processes from shared queue
    for work := range m.workQueue {
        // Download piece
        data, err := m.downloadPiece(work)

        // Send result
        m.resultQueue <- PieceResult{
            Index: work.Index,
            Data:  data,
            Err:   err,
        }
    }
}
```

**Purpose**: N workers pulling from shared work queue provides:
- Load balancing (fast workers get more work)
- Resource limiting (only N concurrent downloads)
- Graceful completion (range over channel)

**Interview Point**: "Worker pool pattern prevents resource exhaustion. Instead of spawning 1000s of goroutines for 1000s of pieces, we use 20 workers that process pieces sequentially from a queue."

---

## 4. sync.WaitGroup

### Location: `internal/downloader/manager.go`

```go
var wg sync.WaitGroup

// Starting workers
for i := 0; i < m.numWorkers; i++ {
    wg.Add(1)
    go func(workerID int) {
        defer wg.Done()
        m.runWorker(workerID)
    }(i)
}

// Wait for all workers to finish
go func() {
    wg.Wait()
    close(m.resultQueue) // Close result channel when done
}()
```

**Purpose**: Coordinate worker completion. Only close `resultQueue` after all workers are done.

**Interview Point**: "WaitGroup tracks when all workers complete. Critical for knowing when to close the result channel - closing too early would lose results, never closing would deadlock the receiver."

---

### Location: `cmd/main.go`

```go
// Waiting for concurrent peer connections
var wg sync.WaitGroup
for i := 0; i < connectLimit; i++ {
    wg.Add(1)
    go func(p tracker.Peer, index int) {
        defer wg.Done()
        // Connect to peer
    }(peers[i], i)
}
wg.Wait() // Wait for all connection attempts
```

**Purpose**: Wait for all concurrent operations before proceeding.

---

## 5. sync.Mutex

### Location: `internal/downloader/manager.go`

```go
type Manager struct {
    pieces []Piece
    mu     sync.Mutex
}

// Protecting shared piece data
m.mu.Lock()
m.pieces[result.Index].Data = result.Data
m.mu.Unlock()
```

**Purpose**: Protect shared `pieces` slice from concurrent writes.

**Interview Point**: "Mutex protects the pieces slice. Multiple workers might try to update different pieces simultaneously, so we lock during writes."

---

### Location: `internal/filewriter/writer.go`

```go
type Writer struct {
    writeCount int
    mu         sync.Mutex
}

// Thread-safe counter
w.mu.Lock()
w.writeCount++
w.mu.Unlock()
```

**Purpose**: Protect non-atomic write counter.

**Note**: File writing itself uses `WriteAt` which is thread-safe without locking.

---

### Location: `cmd/main.go` (Phase 3 test)

```go
var mu sync.Mutex
successfulConns := 0

// In goroutine
mu.Lock()
successfulConns++
mu.Unlock()
```

**Purpose**: Protect shared counter incremented by multiple goroutines.

---

## 6. sync/atomic

### Location: `internal/downloader/manager.go`

```go
type Manager struct {
    downloaded int32 // Atomic counter
}

// Atomic increment (lock-free)
atomic.AddInt32(&m.downloaded, 1)

// Atomic read
count := atomic.LoadInt32(&m.downloaded)
```

**Purpose**: Lock-free increment of download counter. Much faster than mutex for simple operations.

**Interview Point**: "Atomic operations are lock-free and much faster than mutexes for simple counters. No context switching or blocking."

---

### Location: `internal/progress/progress.go`

```go
type Tracker struct {
    totalPieces      int32  // Atomic
    totalBytes       int64  // Atomic
    downloadedPieces int32  // Atomic
    downloadedBytes  int64  // Atomic
}

// Atomic increment from any goroutine
atomic.AddInt32(&t.downloadedPieces, 1)
atomic.AddInt64(&t.downloadedBytes, int64(pieceSize))

// Atomic reads
downloaded := atomic.LoadInt32(&t.downloadedPieces)
bytes := atomic.LoadInt64(&t.downloadedBytes)
```

**Purpose**: Progress tracking from multiple workers without locks.

**Interview Point**: "Progress tracker uses atomic operations exclusively. Workers can update progress from any goroutine without blocking each other. Essential for real-time progress display."

---

## 7. Concurrent File I/O

### Location: `internal/filewriter/writer.go`

```go
// WriteAt is thread-safe - no mutex needed!
func (w *Writer) WritePiece(pieceIndex int, pieceLength int, data []byte) error {
    offset := int64(pieceIndex) * int64(pieceLength)

    // WriteAt writes at offset without changing file position
    // Safe for concurrent calls from multiple goroutines
    n, err := w.file.WriteAt(data, offset)

    return err
}
```

**Purpose**: Multiple workers can write different pieces to the file simultaneously.

**Interview Point**: "os.File.WriteAt() is designed for concurrent use. Unlike Write(), it doesn't modify the file position, so multiple goroutines can call it safely on different offsets. This is critical for performance - workers don't need to wait for each other."

---

## 📊 Concurrency Flow Diagram

```
Torrent File → Parser → Tracker → Peers
                                     ↓
                          ┌──────────┴──────────┐
                          │   Download Manager   │
                          └──────────┬───────────┘
                                     │
                    ┌────────────────┼────────────────┐
                    ↓                ↓                ↓
              ┌─────────┐      ┌─────────┐    ┌─────────┐
              │ Worker 1│      │ Worker 2│    │ Worker N│ (Goroutines)
              └────┬────┘      └────┬────┘    └────┬────┘
                   │                │              │
                   └────────────────┼──────────────┘
                                    ↓
                           ┌────────────────┐
                           │  Work Queue    │ (Channel)
                           │  (Buffered)    │
                           └────────────────┘
                                    ↓
                    ┌───────────────┼───────────────┐
              Download pieces from peers concurrently
                    │               │               │
                    ↓               ↓               ↓
              Piece 1          Piece 2         Piece N
                    │               │               │
                    └───────────────┼───────────────┘
                                    ↓
                           ┌────────────────┐
                           │ Result Queue   │ (Channel)
                           │ (Unbuffered)   │
                           └────────────────┘
                                    ↓
                          ┌─────────────────┐
                          │  File Writer    │ (WriteAt - concurrent safe)
                          │  Progress       │ (Atomic counters)
                          └─────────────────┘
                                    ↓
                            Downloaded File
```

---

## 🎯 Key Interview Talking Points

### 1. **Why Worker Pool?**
"Instead of spawning a goroutine per piece (potentially 1000s), we use a fixed pool of workers. This:
- Limits concurrent network connections
- Prevents resource exhaustion
- Provides natural back-pressure
- Makes resource usage predictable"

### 2. **Why Channels?**
"Channels provide type-safe, race-condition-free communication:
- No shared memory races
- Clear ownership transfer
- Built-in synchronization
- Range loops for clean iteration"

### 3. **Buffered vs Unbuffered Channels**
"workQueue is buffered (size = number of pieces) because we know all work upfront. Prevents blocking when queuing work. resultQueue is unbuffered to provide back-pressure - if file writer is slow, workers naturally slow down."

### 4. **Mutex vs Atomic**
"Use atomic for simple counters - lock-free, faster. Use mutex for complex operations (updating struct fields, checking-then-updating). In progress tracker, atomic is perfect because we only increment counters."

### 5. **WriteAt for Concurrent I/O**
"WriteAt is specifically designed for concurrent use. Unlike Write(), it:
- Doesn't modify file position
- Works with any offset
- Thread-safe without our locking
- Perfect for out-of-order piece writing"

### 6. **WaitGroup Placement**
"Critical to place wg.Wait() correctly. We wait in a separate goroutine and close resultQueue only after all workers finish. This prevents:
- Deadlock (closing channel too early)
- Lost results (closing before workers send)
- Resource leaks (never closing)"

---

## 📚 Resources

- [Go Concurrency Patterns](https://go.dev/blog/pipelines)
- [Effective Go - Concurrency](https://go.dev/doc/effective_go#concurrency)
- [Go Memory Model](https://go.dev/ref/mem)
- [sync/atomic Package](https://pkg.go.dev/sync/atomic)

---

**Summary**: This project demonstrates 7 core concurrency patterns in a real-world application. Each pattern has a specific purpose and shows production-level Go concurrent programming.
