# Go BitTorrent Client — Complete Implementation Reference

> Personal revision doc. Covers every file, every function, every design decision.
> Read this top to bottom to reconstruct the full mental model without touching the code.

## Code Quality Fixes Applied

| Issue | File | Fix |
|---|---|---|
| `rand.Seed(time.Now().UnixNano())` deprecated since Go 1.20 | `worker.go` | Removed `init()` entirely — global rand is auto-seeded |
| `CollectResults`, `GetDownloadedCount`, `AssembleFile` — dead code never called | `manager.go` | Removed all three functions |
| `Piece.Data []byte`, `Manager.downloaded int32`, `Manager.mu sync.Mutex` — only used by dead code | `manager.go` | Removed all three fields |
| Custom `min(a, b int)` wrapper — builtin since Go 1.21 | `main.go` | Removed, uses builtin directly |

---

## Table of Contents

1. [Project Structure](#1-project-structure)
2. [BitTorrent Protocol Primer](#2-bittorrent-protocol-primer)
3. [Entry Point — cmd/main.go](#3-entry-point--cmdmaingo)
4. [Step 1 — Torrent Parser](#4-step-1--torrent-parser)
5. [Step 2 — Tracker Communication](#5-step-2--tracker-communication)
6. [Step 3 — Peer Connection & Handshake](#6-step-3--peer-connection--handshake)
7. [Step 4 — Download Orchestrator](#7-step-4--download-orchestrator)
8. [Step 5 — Download Manager & Worker Pool](#8-step-5--download-manager--worker-pool)
9. [Step 6 — Worker: Piece Download Loop](#9-step-6--worker-piece-download-loop)
10. [Step 7 — File Writer](#10-step-7--file-writer)
11. [Step 8 — Progress Tracker](#11-step-8--progress-tracker)
12. [The Two-Queue Mechanism — Deep Dive](#12-the-two-queue-mechanism--deep-dive)
13. [Idempotency & Resume Logic](#13-idempotency--resume-logic)
14. [Retry & Re-announce Loop](#14-retry--re-announce-loop)
15. [Concurrency Safety Summary](#15-concurrency-safety-summary)
16. [Tests](#16-tests)
17. [End-to-End Flow Diagram](#17-end-to-end-flow-diagram)
18. [Key Interview Questions & Answers](#18-key-interview-questions--answers)

---

## 1. Project Structure

```
torrent-client/
├── cmd/
│   └── main.go                      ← Entry point, CLI flags, wires everything together
├── internal/
│   ├── torrent/
│   │   ├── parser.go                ← Parses .torrent file, calculates InfoHash
│   │   └── parser_test.go
│   ├── tracker/
│   │   └── tracker.go               ← HTTP tracker announce, peer list parsing
│   ├── peer/
│   │   └── peer.go                  ← TCP connection, BitTorrent handshake, message I/O
│   ├── downloader/
│   │   ├── orchestrator.go          ← Top-level download coordinator, retry loop
│   │   ├── manager.go               ← Worker pool, work/result channels
│   │   └── worker.go                ← Per-piece download logic
│   ├── filewriter/
│   │   └── writer.go                ← Thread-safe disk writes, resume support
│   └── progress/
│       ├── progress.go              ← Atomic progress tracking, ETA
│       └── progress_test.go
└── go.mod                           ← Only external dep: jackpal/bencode-go
```

**Dependency rule:** packages only import inward. `downloader` knows about `peer`, `tracker`, `torrent`. Nothing imports `downloader`. This keeps the dependency graph acyclic.

---

## 2. BitTorrent Protocol Primer

Before reading the code, understand what BitTorrent actually does.

### What is a .torrent file?

A `.torrent` file is a bencoded dictionary. Bencode is a simple encoding:
- String: `4:spam` → "spam" (length prefix + colon + data)
- Integer: `i42e` → 42
- List: `l4:spami42ee` → ["spam", 42]
- Dict: `d3:key5:valuee` → {"key": "value"}

A `.torrent` contains:
```
{
  "announce": "http://tracker.example.com/announce",
  "info": {
    "name": "ubuntu.iso",
    "length": 1073741824,     ← total file size in bytes
    "piece length": 524288,   ← each piece is 512KB
    "pieces": "<raw 20-byte SHA1 hashes concatenated>"
  }
}
```

### What is a piece?

The file is split into fixed-size chunks called **pieces** (typically 256KB–2MB each).
Each piece has a SHA-1 hash stored in the `.torrent` file. After downloading a piece, you verify it matches — if not, you discard and re-download from a different peer.

```
File:   [piece 0][piece 1][piece 2]...[piece N-1]
         512KB    512KB    512KB        ≤512KB (last piece is smaller)
```

### What is the InfoHash?

SHA-1 hash of the raw bencoded bytes of the `info` dictionary. This is the torrent's unique ID — used when contacting the tracker and during peer handshakes to confirm both sides are talking about the same torrent.

### What is a tracker?

An HTTP server that knows which peers are downloading a given torrent. You announce yourself with the InfoHash and get back a list of peer IP:port pairs.

### What is a peer?

Another BitTorrent client. You connect to it over TCP, perform a handshake, and download pieces from it.

### What is the peer protocol?

After handshake, peers communicate with length-prefixed messages:
```
<4 bytes: message length><1 byte: message ID><payload>
```
Key message IDs:
| ID | Name | Direction | Meaning |
|---|---|---|---|
| 0 | Choke | peer → you | Stop sending requests |
| 1 | Unchoke | peer → you | You may now send requests |
| 2 | Interested | you → peer | I want data from you |
| 4 | Have | peer → you | I now have piece N |
| 5 | Bitfield | peer → you | Here's which pieces I have |
| 6 | Request | you → peer | Send me block at (index, offset, length) |
| 7 | Piece | peer → you | Here is a block of data |

### What is a block?

Pieces are further split into **16KB blocks** for requests. You can't request a whole piece at once — you request it 16KB at a time, then reassemble.

```
Piece 3 (512KB):
  Request(index=3, begin=0,     length=16384)  → Block 0
  Request(index=3, begin=16384, length=16384)  → Block 1
  ...
  Request(index=3, begin=...,   length=...)    → Block 31
  Assemble → 512KB → SHA1 verify
```

---

## 3. Entry Point — `cmd/main.go`

### CLI Flags

```go
numWorkers := flag.Int("workers", 50, "...")   // default 50 goroutines
outputDir  := flag.String("output", ".", "...") // where to save the file
verbose    := flag.Bool("verbose", false, "...") // extra debug output
```

Usage: `torrent-client -workers 50 -output ./downloads ubuntu.torrent`

### Peer ID Generation

```go
func generatePeerID() [20]byte {
    var peerID [20]byte
    copy(peerID[:], "-GO0001-")  // 8-byte client identifier prefix
    rand.Read(peerID[8:])        // 12 random bytes
    return peerID
}
```

Every BitTorrent client has a 20-byte peer ID. Convention is `-XX0001-` prefix where XX is client code. Ours is `-GO0001-`. The same peer ID is used for both the tracker announce and the peer handshake — so the tracker sees a consistent client identity across re-announces.

### Main flow (4 steps)

```
1. torrent.ParseTorrentFile(path)          → TorrentMeta
2. tracker.GetPeersWithPeerID(meta, id)    → []Peer
3. downloader.NewOrchestrator(...)
4. orchestrator.Download()
```

`min()` is a builtin since Go 1.21 — the custom wrapper that existed in this file was removed. The project uses go 1.24.5 so the builtin is available directly.

---

## 4. Step 1 — Torrent Parser

**File:** `internal/torrent/parser.go`

### TorrentMeta struct

```go
type TorrentMeta struct {
    Announce    string     // tracker URL
    Name        string     // output filename
    Length      int        // total file size in bytes
    PieceLength int        // standard piece size (all pieces except last)
    Pieces      [][]byte   // Pieces[i] = 20-byte SHA1 hash for piece i
    InfoHash    [20]byte   // SHA1 of raw bencoded info dict
}
```

### ParseTorrentFile — two-pass design

**Pass 1** — library decode (line 49):
```go
var torrent bencodeTorrent
bencode.Unmarshal(file, &torrent)
```
Gets you the structured fields: Announce, Name, Length, PieceLength, Pieces string.

**Pass 2** — manual byte extraction for InfoHash (line 55):
```go
infoHash, err := calculateInfoHash(path)
```
Why can't the library do this? Because `Unmarshal` decodes into a Go struct — you get the *values* but the original bytes are gone. The InfoHash must be `SHA1(raw bencoded info bytes)`. Re-encoding the struct would not produce identical bytes (key ordering, whitespace). So you must extract the raw bytes directly.

### calculateInfoHash → extractInfoDictBytes

This is the most custom code in the project. The function manually walks the `.torrent` file bytes:

```
data = entire file bytes
      ↓
extractInfoDictBytes(data):
  1. Read first byte → must be 'd' (root dict)
  2. Loop: read key string → is it "info"?
     - No: skipBencodeValue(r) and continue
     - Yes: record current byte position
            findMatchingEnd(data[infoStart:]) → find where info dict ends
            return data[infoStart : infoStart+end]
```

**readBencodeString** (line 161): reads `<digits>:<data>`. Accumulates digit bytes until `:`, converts to int, reads that many bytes.

**skipBencodeValue** (line 191): advances the reader past one bencode value without storing it. Handles all 4 types:
- Integer `i...e`: read until `e`
- String `N:data`: read length, seek N bytes forward
- List `l...e`: recursively skip elements until `e`
- Dict `d...e`: recursively skip key-value pairs until `e`

**findMatchingEnd** (line 274): given bytes starting with `d`, finds the matching `e` at the correct nesting depth. Uses `bencodeValueLength` to skip each key and value by their actual byte length — this is critical because the `pieces` field contains raw binary bytes that may include `d` and `e` characters. If you just scanned for `e`, you'd stop too early inside the pieces data. By computing exact lengths, you skip over string contents entirely.

**bencodeValueLength** (line 303): returns the total byte length of one bencode value (including its delimiters). Recursive for lists and dicts.

### splitPieces (line 79)

```go
// pieces string = 20-byte SHA1 hashes concatenated
// "12345678901234567890abcdefghijklmnopqrst" → 2 pieces of 20 bytes each

numPieces := len(piecesStr) / 20
pieces[i] = []byte(piecesStr[i*20 : (i+1)*20])
```

Validation: `len(piecesStr) % 20 != 0` → error. If the string isn't a multiple of 20, the file is corrupt.

### PieceSize (line 363)

```go
func (t *TorrentMeta) PieceSize(index int) int {
    if index == len(t.Pieces)-1 {
        remainder := t.Length % t.PieceLength
        if remainder != 0 {
            return remainder
        }
    }
    return t.PieceLength
}
```

All pieces are `PieceLength` bytes. **Except the last piece**, which is `Length % PieceLength` bytes (if that's non-zero). Example: 1000-byte file, 300-byte pieces → 4 pieces: [300, 300, 300, 100].

This is used everywhere: when allocating the `pieceData` buffer, when requesting blocks, when verifying hashes.

---

## 5. Step 2 — Tracker Communication

**File:** `internal/tracker/tracker.go`

### GetPeersWithPeerID (line 49)

Called from main with the same peerID generated at startup:
```go
peers, interval, err := tracker.GetPeersWithPeerID(meta, peerID)
```

Returns:
- `[]Peer` — list of IP:port pairs
- `int` — tracker's requested re-announce interval in seconds
- `error`

### buildTrackerURLWithPeerID (line 103)

Builds a GET URL like:
```
http://tracker.example.com/announce?
  info_hash=%01%02...   ← raw 20-byte InfoHash, URL-encoded
  &peer_id=-GO0001-...  ← raw 20-byte peer ID, URL-encoded
  &port=6881
  &uploaded=0
  &downloaded=0
  &left=1073741824      ← bytes remaining (= total size, since we haven't started)
  &compact=1            ← ask for compact peer format (6 bytes/peer instead of dict)
```

The `info_hash` and `peer_id` are raw binary bytes passed as strings in `url.Values`. Go's `url.Values.Encode()` percent-encodes them correctly.

### HTTP Request & Response

```go
resp, err := http.Get(trackerURL)
bencode.Unmarshal(resp.Body, &trackerResp)
```

Tracker response is bencoded:
```
{
  "interval": 1800,   ← re-announce every 30 minutes
  "peers": "<compact binary>"
}
```

### parsePeers (line 141)

Compact format: 6 bytes per peer.
```
bytes 0-3: IPv4 address (big-endian)
bytes 4-5: port (big-endian uint16)

"1.2.3.4:6881" = [0x01, 0x02, 0x03, 0x04, 0x1A, 0xE1]
```

```go
ipStr := fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3])
port := binary.BigEndian.Uint16([]byte(peersData[offset+4 : offset+6]))
```

Validation: `len(peersData) % 6 != 0` → error.

---

## 6. Step 3 — Peer Connection & Handshake

**File:** `internal/peer/peer.go`

### ConnectToPeer (line 59)

```go
conn, err := net.DialTimeout("tcp", peer.String(), 3*time.Second)
```

3-second dial timeout. If a peer is unreachable, we fail fast and try another.

### PerformHandshake (line 76)

**Send** (68 bytes total):
```
[0]        1 byte  = 19 (length of "BitTorrent protocol")
[1..19]   19 bytes = "BitTorrent protocol"
[20..27]   8 bytes = 0x00 (reserved, all zeros)
[28..47]  20 bytes = InfoHash
[48..67]  20 bytes = our PeerID
```

**Receive** same format back.

**Verify**: `hs.InfoHash == our InfoHash`. If they don't match, the peer is serving a different torrent. Close and try another peer.

**Deadline**: `conn.SetDeadline(time.Now().Add(5 * time.Second))`. If handshake takes more than 5 seconds, the connection is bad. Reset to zero after handshake so it doesn't affect later reads.

### ConnectAndHandshake (line 236)

Convenience wrapper: connect + handshake in one call. Returns a `*Connection` struct:
```go
type Connection struct {
    Conn     net.Conn
    Peer     tracker.Peer
    Choked   bool      // starts true — peer starts choked
    Bitfield []byte
    mu       sync.Mutex
}
```

`Choked: true` by default — the protocol requires you to wait for an Unchoke message before sending requests.

### ReadMessage (line 158)

```
Read 4 bytes → message length (uint32, big-endian)
If length == 0 → keep-alive, return nil
Read 1 byte  → message ID
Read length-1 bytes → payload
```

10-second read deadline per message. If a peer goes silent, we time out and fail.

### SendMessage (line 197)

```
4 bytes: length = 1 + len(payload)
1 byte:  message ID
N bytes: payload
```

### SendRequest (line 223)

```go
payload = [4 bytes index][4 bytes begin][4 bytes length]
```

Requests a 16KB block at `begin` offset within piece `index`.

---

## 7. Step 4 — Download Orchestrator

**File:** `internal/downloader/orchestrator.go`

The orchestrator is the top-level coordinator. It owns the retry loop, the `remaining` set, and the file writer. Workers never touch the file directly.

### Orchestrator struct

```go
type Orchestrator struct {
    meta       *torrent.TorrentMeta
    peers      []tracker.Peer
    peerID     [20]byte
    outputPath string
    numWorkers int
}
```

### Download() — the main loop

#### Setup

```go
progressTracker := progress.NewTracker(numPieces, totalBytes)
writer, err := filewriter.NewWriterForResume(outputPath, totalSize, pieceLength)
```

`NewWriterForResume` opens the file without truncating if it exists — enabling resume.

#### remaining map

```go
remaining := make(map[int]struct{})
for i := 0; i < totalPieces; i++ {
    remaining[i] = struct{}{}
}
```

`remaining` is the single source of truth for what still needs to be downloaded. A piece is removed from it when successfully written to disk.

#### Idempotency check (before first download round)

```go
for i := 0; i < totalPieces; i++ {
    offset := int64(i) * int64(meta.PieceLength)
    data, err := writer.ReadAt(offset, pieceLen)
    if err != nil { continue }
    if VerifyPiece(data, meta.Pieces[i]) != nil { continue }
    delete(remaining, i)  // already good on disk
}
```

On a fresh download, all pieces are uninitialized zeros — they won't pass SHA1 verification, so nothing is removed. On a resume, valid pieces are skipped.

#### Retry loop

```go
for round := 0; ; round++ {
    if len(remaining) == 0 { break }
    ...
    pieceIndices := []int of remaining keys
    manager := NewManager(meta, peers, peerID, numWorkers, pieceIndices)
    manager.Start()
    
    for result := range manager.resultQueue {
        if result.Err != nil { continue } // stays in remaining
        delete(remaining, result.Index)
        writer.WritePiece(result.Index, result.Data)
        progressTracker.AddPiece(len(result.Data))
    }
    // manager.resultQueue closed → loop exits → check remaining
}
```

Each round creates a fresh Manager with only the not-yet-downloaded pieces. Failed pieces stay in `remaining` and are retried next round.

#### Re-announce logic

```go
const minIntervalSec = 5
const retryDelaySec = 30

if time.Since(lastAnnounceTime) >= announceIntervalSec {
    peers, announceIntervalSec, err = tracker.GetPeersWithPeerID(meta, peerID)
    lastAnnounceTime = time.Now()
}
```

Only contacts the tracker again when the tracker's own requested interval has elapsed. This prevents rate-limiting or bans. The `minIntervalSec = 5` floor ensures a tracker returning `interval=0` doesn't spam.

---

## 8. Step 5 — Download Manager & Worker Pool

**File:** `internal/downloader/manager.go`

### The Three Structs

```go
type PieceWork struct {
    Index  int    // which piece
    Hash   []byte // expected SHA1
    Length int    // how many bytes
}
// Enqueued into workQueue. No actual data — just a description of work.

type PieceResult struct {
    Index int
    Data  []byte // downloaded+verified bytes
    Err   error
}
// Sent into resultQueue after download attempt.

// Piece is used internally by the Manager to hold piece metadata.
// It does NOT hold downloaded data — results flow through PieceResult.
type Piece struct {
    Index  int
    Hash   []byte
    Length int
}

type Manager struct {
    workQueue    chan PieceWork   // buffered
    resultQueue  chan PieceResult // unbuffered
    pieces       []Piece         // metadata only, no downloaded data
    ...
}
```

### NewManager (line 48)

```go
queueCap := len(pieceIndices)
if queueCap < numWorkers*2 {
    queueCap = numWorkers * 2
}
workQueue = make(chan PieceWork, queueCap)
resultQueue = make(chan PieceResult) // no capacity = unbuffered
```

The Manager struct holds only what is needed: channel handles, piece metadata, peers, and peerID. It has no `mu sync.Mutex` or `downloaded int32` — those were removed along with the dead code that used them.

`workQueue` capacity = at least `numWorkers*2`. This ensures `Start()` can fill the queue without blocking even if workers haven't started yet.

`resultQueue` capacity = 0 (unbuffered). Workers block on send until orchestrator reads. This is intentional backpressure — see Section 12.

### Start() (line 80)

```go
// Phase 1: fill workQueue
for _, i := range indices {
    m.workQueue <- PieceWork{...}
}
close(m.workQueue) // signal: no more work

// Phase 2: spawn workers
var wg sync.WaitGroup
for i := 0; i < numWorkers; i++ {
    wg.Add(1)
    go func(id int) {
        defer wg.Done()
        m.runWorker(id)
    }(i)
}

// Phase 3: close resultQueue when all workers done
go func() {
    wg.Wait()
    close(m.resultQueue)
}()
```

**Why close workQueue before workers start?** It's fine — channels are safe to close before readers start. Workers will drain it normally, then exit their `range` loop when it's empty+closed.

**Why the separate goroutine for close(resultQueue)?** If it were inline after `wg.Wait()`, it would deadlock: `wg.Wait()` needs workers to finish → workers need `resultQueue` to be read → reading happens in orchestrator → orchestrator is blocked waiting for `Start()` to return. The goroutine breaks this cycle.

### VerifyPiece (line 120)

```go
func VerifyPiece(data []byte, expectedHash []byte) error {
    hash := sha1.Sum(data)
    if !bytes.Equal(hash[:], expectedHash) {
        return fmt.Errorf("hash mismatch: expected %x, got %x", expectedHash, hash[:])
    }
    return nil
}
```

Called in two places:
1. Worker after downloading a piece — if mismatch, try another peer
2. Orchestrator idempotency check — verifying pieces already on disk

---

## 9. Step 6 — Worker: Piece Download Loop

**File:** `internal/downloader/worker.go`

### runWorker (line 22)

```go
func (m *Manager) runWorker(workerID int) {
    for work := range m.workQueue {
        data, err := m.downloadPiece(work)
        m.resultQueue <- PieceResult{Index: work.Index, Data: data, Err: err}
    }
    // range exits when workQueue is closed AND drained
}
```

Simple: pull work, do work, push result. Repeat until no more work.

### downloadPiece (line 38)

Tries up to `maxRetries = 10` random peers:

```go
for attempt := 0; attempt < 10; attempt++ {
    peerIndex := rand.Intn(len(peers))
    data, err := downloadPieceFromPeer(peers[peerIndex], work)
    if err != nil { lastErr = err; continue }
    if VerifyPiece(data, work.Hash) != nil { lastErr = ...; continue }
    return data, nil  // success
}
return nil, fmt.Errorf("failed after 10 attempts: %w", lastErr)
```

Random peer selection — no sticky sessions, no preference ordering. Each attempt picks a fresh random peer. If the same peer is picked twice and fails twice, that's fine — 10 attempts with N peers gives good coverage.

`rand.Intn` uses Go's global random source, which is auto-seeded since Go 1.20. There is no `rand.Seed` call — it was removed as deprecated.

### downloadPieceFromPeer (line 72)

This is the core BitTorrent protocol exchange:

**Step 1 — Connect & Handshake**
```go
conn, err := peer.ConnectAndHandshake(peerAddr, meta.InfoHash, peerID)
defer conn.Close()
```

**Step 2 — Send Interested**
```go
peer.SendInterested(conn.Conn)
// Message: length=1, ID=2, no payload
```

Tells the peer "I want data from you." Without this, peers won't unchoke you.

**Step 3 — Wait for Unchoke**
```go
for i := 0; i < 10; i++ {
    msg, _ := peer.ReadMessage(conn.Conn)
    if msg.ID == 1 { unchoked = true; break }    // Unchoke
    if msg.ID == 5 { continue }                  // Bitfield — skip
}
if !unchoked { return nil, error }
```

Peers start choked. They unchoke you when they're willing to serve you. We read up to 10 messages looking for it. Bitfield messages (telling you what pieces they have) are silently skipped — in a real client you'd track this to know which peers have which pieces.

**Step 4 — Block download loop**
```go
pieceData := make([]byte, work.Length)
downloaded := 0

for downloaded < work.Length {
    requestSize := min(blockSize, work.Length - downloaded) // 16KB or less

    peer.SendRequest(conn.Conn, index, downloaded, requestSize)

    // Read until we get a Piece message (ID=7)
    for {
        msg = peer.ReadMessage(conn.Conn)
        if msg.ID == 7 { break }
        if msg.ID == 0 { return error("choked") } // peer changed its mind
        // other messages (Have, etc.) — skip
    }

    // Parse piece message payload: [4B index][4B begin][data]
    receivedIndex := BigEndian.Uint32(payload[0:4])
    receivedBegin := BigEndian.Uint32(payload[4:8])
    blockData     := payload[8:]

    // Sanity checks
    if receivedIndex != work.Index { return error }
    if receivedBegin != downloaded  { return error }

    copy(pieceData[downloaded:], blockData)
    downloaded += len(blockData)
}
```

The loop sends one request, waits for the response, verifies the offset matches, copies the data, advances `downloaded`. Continues until all bytes of the piece are received.

**Why send one request at a time?** A real client would pipeline multiple requests (send several requests before waiting for responses) for better throughput. This client keeps it simple: one request → one response → next request. Simpler to reason about, but slower.

---

## 10. Step 7 — File Writer

**File:** `internal/filewriter/writer.go`

### NewWriterForResume (line 43)

```go
fi, err := os.Stat(filePath)
if err == nil {
    // File exists
    file, _ = os.OpenFile(filePath, os.O_RDWR, 0666)
    if fi.Size() != totalSize {
        file.Truncate(totalSize) // resize if needed
    }
    return &Writer{...}
}
if os.IsNotExist(err) {
    return NewWriter(filePath, totalSize, pieceLength) // create fresh
}
```

Resume logic: if the file exists and is the right size, open without truncating. Existing pieces will be verified by the idempotency check in the orchestrator. If wrong size, resize. If doesn't exist, create.

### NewWriter (line 20)

```go
file, _ = os.Create(filePath)   // creates or truncates
file.Truncate(totalSize)         // pre-allocate full size on disk
```

Pre-allocating via `Truncate` is important. It creates a sparse file (on most filesystems) at the full final size. This means `WriteAt` can write to any offset immediately — there's no need to write pieces in order.

### WritePiece (line 86)

```go
offset := int64(pieceIndex) * int64(w.pieceLength)
n, err := w.file.WriteAt(data, offset)
```

**Critical detail:** uses `w.pieceLength` (the standard piece size) for offset calculation, **not** `len(data)`. The last piece is smaller than `pieceLength` but still lives at `index * pieceLength` in the file. If you used `len(data)` for the last piece, it'd be at the wrong offset.

**Why WriteAt is thread-safe:** `WriteAt` writes at a fixed position without moving the file's seek pointer. Two goroutines calling `WriteAt` at different offsets (different pieces) do not interfere. The kernel serializes the underlying writes safely. This is why the orchestrator can call `WritePiece` without a mutex — pieces never overlap.

The `sync.Mutex` in `Writer` only protects the `writeCount` counter, not the actual I/O.

### ReadAt (line 71)

```go
buf := make([]byte, length)
n, err := w.file.ReadAt(buf, offset)
```

Used by the idempotency check in the orchestrator to read existing piece data for SHA1 verification.

### Close (line 111)

```go
w.file.Sync()  // flush OS write buffers to disk
w.file.Close()
```

`Sync()` before `Close()` ensures data is physically on disk, not just in kernel page cache. Prevents data loss if the system crashes immediately after the program exits.

---

## 11. Step 8 — Progress Tracker

**File:** `internal/progress/progress.go`

### Thread safety with atomics

```go
type Tracker struct {
    downloadedPieces int32  // atomic
    downloadedBytes  int64  // atomic
    totalPieces      int32  // atomic (read-only after init)
    totalBytes       int64  // atomic (read-only after init)
    startTime        time.Time
}
```

`AddPiece` uses `atomic.AddInt32` / `atomic.AddInt64`:
```go
func (t *Tracker) AddPiece(pieceSize int) {
    atomic.AddInt32(&t.downloadedPieces, 1)
    atomic.AddInt64(&t.downloadedBytes, int64(pieceSize))
}
```

These are CPU-level atomic operations. No mutex needed. Any number of goroutines can call `AddPiece` concurrently without data races.

In this codebase, `AddPiece` is only called from the orchestrator's single-goroutine result loop — so the atomics aren't strictly needed. But it's correct defensive practice.

### GetStats (line 51)

```go
speed = downloadedBytes / elapsed.Seconds()
eta   = remainingBytes / speed
```

Speed is average since start (not a rolling window). Simple but accurate enough for a progress display.

### FormatSimple (line 140)

```go
fmt.Sprintf("Progress: %.1f%% (%d/%d pieces)", percent, downloaded, total)
```

Used with `\r` (carriage return without newline) to update the same terminal line in place.

---

## 12. The Two-Queue Mechanism — Deep Dive

This is the core concurrency design of the project.

### Why two queues?

| | `workQueue` | `resultQueue` |
|---|---|---|
| Direction | orchestrator → workers | workers → orchestrator |
| Type | `chan PieceWork` (metadata) | `chan PieceResult` (data) |
| Buffered? | Yes — `max(numPieces, numWorkers*2)` | No — capacity 0 |
| Purpose | Distribute work | Collect results |
| Alternative | Could use shared slice + mutex | Could use shared slice + mutex |

Channels are cleaner than mutex+slice. Each worker independently pulls and pushes without coordinating with other workers.

### Why workQueue is buffered

The orchestrator fills `workQueue` **before** workers start (inside `Start()`). If the channel were unbuffered, the first `workQueue <-` would block forever (no one is reading yet). The buffer must hold all pieces.

### Why resultQueue is unbuffered

**Backpressure.** An unbuffered send blocks the sender until someone receives.

```
Without backpressure (buffered resultQueue):
  50 workers download 50 pieces simultaneously
  Push 50 results to buffered queue
  Pull next 50 pieces from workQueue
  Download 50 more → push 50 more
  → up to numWorkers * 2 pieces in memory at once
  → 50 workers × 2MB piece = 100MB+ in memory before anything is written to disk

With backpressure (unbuffered resultQueue):
  Worker finishes → blocks on resultQueue send
  Orchestrator reads → writes to disk → reads next result
  Worker unblocks → pulls next piece from workQueue
  → at most numWorkers pieces exist in memory (in-flight downloads)
  → disk write speed throttles the whole pipeline
```

### How goroutines wait (parking)

When a goroutine sends to an unbuffered channel with no receiver:

1. Go runtime **suspends** the goroutine — removes it from the run queue
2. Goroutine sits in the channel's internal **send wait queue** — zero CPU usage
3. The OS thread is freed to run other goroutines
4. When orchestrator calls `<-resultQueue`, runtime **wakes** the oldest waiting goroutine
5. Value is transferred directly (hand-off), goroutine resumes

This is not a spin loop. Parked goroutines consume no CPU.

### Multiple goroutines waiting simultaneously

```
Scenario: orchestrator is slow writing to disk

goroutine 0: downloading piece 5...  ← still running
goroutine 1: finished piece 9  → resultQueue <- result9  ← PARKED
goroutine 2: downloading piece 3...  ← still running
goroutine 3: finished piece 1  → resultQueue <- result1  ← PARKED
goroutine 4: downloading piece 7...  ← still running

Channel's send wait queue: [goroutine 1 (result9), goroutine 3 (result1)]

Orchestrator finishes writing piece X, reads resultQueue:
  → wakes goroutine 1 (FIFO), gets result9
  → goroutine 1 pulls next piece from workQueue and starts downloading
  → goroutine 3 still parked waiting

Orchestrator writes piece 9, reads again:
  → wakes goroutine 3, gets result1
  → etc.
```

The pool never fully stops. Some goroutines are parked waiting to push, others are actively downloading. Total in-memory pieces = workers currently downloading + workers parked with completed pieces.

### The WaitGroup + close(resultQueue) pattern

```go
// In Start():
go func() {
    wg.Wait()           // blocks until all workers' runWorker() returns
    close(m.resultQueue) // signals "no more results coming"
}()

// In orchestrator:
for result := range manager.resultQueue {
    // process...
}
// range exits here, when resultQueue is both closed AND drained
```

This goroutine must be separate. If it were inline:
```
// DEADLOCK if done inline:
wg.Wait()             // needs workers to finish
workers need resultQueue to be read
resultQueue is read in orchestrator
orchestrator is blocked waiting for Start() to return
Start() is blocked on wg.Wait()
→ circular wait → deadlock
```

---

## 13. Idempotency & Resume Logic

**Files:** `orchestrator.go` (lines 69-83), `filewriter/writer.go` (NewWriterForResume)

### What idempotency means here

If you run the client twice on the same output file, it should not re-download pieces that are already correct on disk. "Already correct" means the SHA1 of the bytes on disk matches the expected hash from the `.torrent` file.

### How it works

1. `NewWriterForResume`: opens existing file without truncating (preserves existing data)
2. Idempotency loop: for each piece index, read bytes from disk at `offset = i * pieceLength`, run `VerifyPiece`. Pass → remove from `remaining`. Fail → keep in `remaining`.
3. `pieceIndices` passed to Manager = `remaining` keys only. Already-good pieces are never queued.

### Edge cases handled

- **File doesn't exist**: `NewWriterForResume` falls back to `NewWriter` — creates and pre-allocates
- **File exists but wrong size**: `Truncate(totalSize)` resizes it. Old data at valid offsets may still be good; idempotency check will verify
- **Piece is zeroed** (fresh file, undownloaded): SHA1 of all-zeros won't match → stays in `remaining`
- **Piece is corrupt** (partial download, crash mid-write): SHA1 mismatch → stays in `remaining`, gets re-downloaded

---

## 14. Retry & Re-announce Loop

**File:** `orchestrator.go` (lines 89-155)

### Why retry is needed

Not all peers have all pieces. Not all connections succeed. Network is unreliable. A piece that fails once may succeed on the next attempt with a different peer.

### Retry loop structure

```
round 0: try all remaining pieces with current peer list
round 1: sleep 30s → maybe re-announce → try still-remaining pieces
round 2: sleep 30s → maybe re-announce → ...
until remaining is empty
```

### Re-announce timing

```go
const minIntervalSec = 5   // never re-announce faster than every 5s
const retryDelaySec  = 30  // sleep between retry rounds

if time.Since(lastAnnounceTime) >= announceIntervalSec {
    peers, announceIntervalSec, err = tracker.GetPeersWithPeerID(meta, peerID)
}
```

The tracker tells you its preferred re-announce interval (e.g., 1800 seconds = 30 minutes). Respecting this avoids being banned. The `minIntervalSec` floor handles trackers that return `interval=0`.

Re-announcing gives you a fresh peer list — some peers from round 0 may be gone, new peers may have appeared.

### What happens to failed pieces

```go
for result := range manager.resultQueue {
    if result.Err != nil {
        continue  // piece stays in `remaining`
    }
    delete(remaining, result.Index)
    ...
}
```

An error result is silently skipped. The piece index stays in `remaining`. Next round, it gets a new Manager and fresh attempts. The error from the last attempt is logged but not fatal — the download continues.

### Completion check

After the retry loop:
```go
completedCount := totalPieces - len(remaining)
if completedCount < totalPieces {
    return fmt.Errorf("incomplete: got %d/%d", completedCount, totalPieces)
}
```

---

## 15. Concurrency Safety Summary

| Resource | Access pattern | Protection mechanism |
|---|---|---|
| `workQueue` | One writer (Start), many readers (workers) | Channel — goroutine-safe by design |
| `resultQueue` | Many writers (workers), one reader (orchestrator) | Channel — goroutine-safe by design |
| `remaining` map | One reader+writer (orchestrator) | Single goroutine — no protection needed |
| `file.WriteAt` | One writer (orchestrator) | Non-overlapping offsets — no mutex needed |
| `file.ReadAt` | One reader (orchestrator, idempotency check only) | Single goroutine |
| `progressTracker` | `AddPiece` called from orchestrator | `atomic.AddInt32/64` |
| `Writer.writeCount` | Multiple callers possible | `sync.Mutex` |
| `peer.Connection.mu` | Not used in current code | Defined but unused |

**Key insight:** the orchestrator is the only goroutine that touches `remaining`, the writer, and the progress tracker. Workers only touch their own local variables and shared channels. This design avoids almost all locking.

---

## 16. Tests

### `torrent/parser_test.go`

**TestSplitPieces**: table-driven test covering valid input (2 pieces = 40 bytes), invalid length (3 bytes → error), and empty string (0 pieces, no error).

**TestTorrentMetaPieceSize**: verifies the last-piece size logic. For a 1000-byte file with 300-byte pieces: pieces 0-2 return 300, piece 3 returns 100 (= 1000 % 300). Also tests out-of-bounds indices return 0.

**TestNumPieces**: trivial — `len(meta.Pieces)`.

### `progress/progress_test.go`

**TestAddPiece**: basic add, check counts and bytes.

**TestConcurrentAddPiece**: 10 goroutines × 100 adds = 1000 total. Verifies atomics prevent data races. If `AddPiece` used a non-atomic operation, this test would catch it (or the race detector would).

**TestGetPercentComplete**: 0% → 50% → 100% with byte-level tracking.

**TestIsComplete**: checks piece-count based completion.

**TestDownloadSpeed**: adds 1000 bytes after 100ms sleep, checks speed is positive. Wide range accepted due to timing variance.

**TestReset**: add pieces, reset, verify counters back to zero.

**BenchmarkAddPiece / BenchmarkGetStats / BenchmarkConcurrentAddPiece**: performance baselines. `BenchmarkAddPiece` measures atomic add throughput under serial load. The concurrent benchmark measures under contention.

---

## 17. End-to-End Flow Diagram

```
main.go
  │
  ├─ ParseTorrentFile("ubuntu.torrent")
  │     │
  │     ├─ bencode.Unmarshal → Announce, Name, Length, PieceLength, Pieces string
  │     ├─ calculateInfoHash → extractInfoDictBytes → SHA1 → [20]byte InfoHash
  │     └─ splitPieces → [][]byte (one 20-byte hash per piece)
  │
  ├─ generatePeerID → [20]byte "-GO0001-<random>"
  │
  ├─ tracker.GetPeersWithPeerID(meta, peerID)
  │     │
  │     ├─ buildTrackerURL → GET http://tracker/announce?info_hash=...
  │     ├─ http.Get → bencode response
  │     └─ parsePeers → []Peer{IP, Port} (compact 6-byte format)
  │
  └─ orchestrator.Download()
        │
        ├─ filewriter.NewWriterForResume → open or create output file
        │
        ├─ idempotency check
        │     └─ for each piece: ReadAt → VerifyPiece → delete from remaining if good
        │
        └─ retry loop (round 0, 1, 2, ...)
              │
              ├─ NewManager(meta, peers, peerID, numWorkers, remaining pieces)
              │     ├─ workQueue  = buffered chan PieceWork
              │     └─ resultQueue = unbuffered chan PieceResult
              │
              ├─ manager.Start()
              │     ├─ fill workQueue with PieceWork for each remaining piece
              │     ├─ close(workQueue)
              │     ├─ spawn 50 goroutines → runWorker
              │     └─ goroutine: wg.Wait() → close(resultQueue)
              │
              │     ┌─── Worker goroutines (×50) ───────────────────────────┐
              │     │  for work := range workQueue {                        │
              │     │      downloadPiece(work):                             │
              │     │        for attempt := 0..9 {                          │
              │     │          peer = random peer                            │
              │     │          TCP connect (3s timeout)                     │
              │     │          BitTorrent handshake (verify InfoHash)       │
              │     │          SendInterested                                │
              │     │          wait for Unchoke                             │
              │     │          for downloaded < pieceLength:                │
              │     │            SendRequest(index, offset, 16KB)           │
              │     │            ReadMessage until Piece(ID=7)              │
              │     │            copy block → pieceData                     │
              │     │          VerifyPiece (SHA1)                           │
              │     │          if ok: break                                  │
              │     │        }                                               │
              │     │      resultQueue <- PieceResult{Index, Data, Err}     │
              │     │        ↑ blocks here if orchestrator busy writing     │
              │     │  }                                                     │
              │     └────────────────────────────────────────────────────────┘
              │
              ├─ for result := range manager.resultQueue {
              │     if err: continue (stays in remaining)
              │     delete(remaining, result.Index)
              │     writer.WritePiece(index, data)
              │       → WriteAt(data, index * pieceLength)
              │     progressTracker.AddPiece(len(data))
              │     print "\r Progress: X%"
              │   }
              │   // loop exits when resultQueue closed (all workers done)
              │
              ├─ if remaining empty: break
              └─ else: sleep 30s, maybe re-announce, next round
```

---

## 18. Key Interview Questions & Answers

**Q: Why did you write your own bencode parser for the InfoHash instead of using jackpal/bencode-go?**

A: The library decodes bencode into Go structs — you get the field values but lose the original bytes. The InfoHash is `SHA1(raw bencoded info dict bytes)`. Re-encoding the struct back to bencode would not produce identical bytes (key ordering, string encoding differences). To get the exact original bytes, I wrote `extractInfoDictBytes` which walks the file byte-by-byte, locates the `info` key, and slices out the raw bytes between the dict's `d` and matching `e`.

**Q: How does concurrent writing work without corrupting the file?**

A: `file.WriteAt(data, offset)` writes at a fixed byte offset without moving the file's seek pointer. Two goroutines writing to different offsets (different pieces) never interfere. The kernel serializes the actual I/O safely. Pieces are non-overlapping because each lives at `index * pieceLength`, and no two pieces have the same index. No mutex is needed for the I/O itself.

**Q: Why is resultQueue unbuffered?**

A: Backpressure. If it were buffered, workers could dump all their results into the queue before the orchestrator writes any of them to disk. On a slow disk with 50 fast-downloading workers, you'd accumulate up to `numWorkers * pieceSize` bytes in memory before anything hits disk. With an unbuffered channel, each worker blocks after finishing a piece until the orchestrator is ready to write it. Disk write speed naturally throttles the download rate, bounding memory usage.

**Q: What happens when a piece hash doesn't match?**

A: `VerifyPiece` returns an error. In `downloadPiece`, the current attempt is abandoned and the loop continues to the next attempt with a different random peer. After 10 failed attempts, the piece is returned as an error result. In the orchestrator, error results are skipped — the piece stays in the `remaining` map. The next retry round creates a new Manager with that piece re-queued, giving it another 10 attempts.

**Q: How does resume work?**

A: Two parts. First, `NewWriterForResume` opens an existing file without truncating (preserving existing data). Second, before the first download round, the orchestrator reads each piece from disk at its expected offset and runs SHA1 verification. Pieces that pass are removed from the `remaining` set and never queued. Pieces that fail (corrupt, zero-initialized, or from a truncated file) stay in `remaining` and are downloaded normally.

**Q: Why does the worker pool use two separate channels instead of a shared slice with a mutex?**

A: Channels make the coordination structure explicit and deadlock-free by construction. With a shared work slice: workers need a mutex to pop items, need to signal when done, need another mechanism to collect results — more code, more potential for bugs. With channels: `range workQueue` automatically distributes work and exits when done. `resultQueue <-` automatically signals completion. `close(workQueue)` naturally terminates workers. The concurrency intent is visible in the channel operations themselves.

**Q: What is the WaitGroup for? Why not just close resultQueue directly?**

A: The WaitGroup tracks when all workers have finished their `runWorker` goroutine (i.e., have both finished downloading AND sent their result). `close(resultQueue)` must happen only after all workers are done sending — closing a channel that a goroutine is still trying to send to causes a panic. `wg.Wait()` provides the synchronization point: it returns exactly when the last worker exits `runWorker`, at which point it's safe to close `resultQueue`.

**Q: Why do you respect the tracker's re-announce interval?**

A: Trackers rate-limit clients. Announcing too frequently can get your IP temporarily banned, meaning you lose access to the peer list entirely. The tracker's `interval` field in its response tells you the minimum time to wait before re-announcing. We enforce a floor of `minIntervalSec = 5` as well, to handle trackers that return `interval=0` which would otherwise cause continuous spam.
