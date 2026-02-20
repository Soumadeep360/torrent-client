package torrent

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"os"

	"github.com/jackpal/bencode-go"
)

// TorrentMeta represents the metadata extracted from a .torrent file
type TorrentMeta struct {
	Announce    string     // Tracker URL
	Name        string     // File or directory name
	Length      int        // Total length in bytes (single file mode)
	PieceLength int        // Length of each piece in bytes
	Pieces      [][]byte   // List of SHA1 hashes for each piece
	InfoHash    [20]byte   // SHA1 hash of the info dictionary
}

// bencodeInfo represents the 'info' dictionary in the torrent file
type bencodeInfo struct {
	Name        string `bencode:"name"`
	Length      int    `bencode:"length"`
	PieceLength int    `bencode:"piece length"`
	Pieces      string `bencode:"pieces"` // Concatenated 20-byte SHA1 hashes
}

// bencodeTorrent represents the top-level structure of a .torrent file
type bencodeTorrent struct {
	Announce string      `bencode:"announce"`
	Info     bencodeInfo `bencode:"info"`
}

// ParseTorrentFile parses a .torrent file and extracts metadata
func ParseTorrentFile(path string) (*TorrentMeta, error) {
	// Open the torrent file
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open torrent file: %w", err)
	}
	defer file.Close()

	// Decode the bencode data
	var torrent bencodeTorrent
	if err := bencode.Unmarshal(file, &torrent); err != nil {
		return nil, fmt.Errorf("failed to decode bencode: %w", err)
	}

	// Calculate info hash (SHA1 of the bencoded info dictionary)
	// We need to re-open and parse to get raw info dict
	infoHash, err := calculateInfoHash(path)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate info hash: %w", err)
	}

	// Split the pieces string into individual 20-byte hashes
	pieces, err := splitPieces(torrent.Info.Pieces)
	if err != nil {
		return nil, fmt.Errorf("failed to split pieces: %w", err)
	}

	meta := &TorrentMeta{
		Announce:    torrent.Announce,
		Name:        torrent.Info.Name,
		Length:      torrent.Info.Length,
		PieceLength: torrent.Info.PieceLength,
		Pieces:      pieces,
		InfoHash:    infoHash,
	}

	return meta, nil
}

// splitPieces splits the concatenated piece hashes into individual 20-byte slices
func splitPieces(piecesStr string) ([][]byte, error) {
	const hashLen = 20 // SHA1 hash length

	if len(piecesStr)%hashLen != 0 {
		return nil, fmt.Errorf("invalid pieces length: %d (not divisible by %d)", len(piecesStr), hashLen)
	}

	numPieces := len(piecesStr) / hashLen
	pieces := make([][]byte, numPieces)

	for i := 0; i < numPieces; i++ {
		start := i * hashLen
		end := start + hashLen
		pieces[i] = []byte(piecesStr[start:end])
	}

	return pieces, nil
}

// calculateInfoHash computes the SHA1 hash of the info dictionary
func calculateInfoHash(path string) ([20]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return [20]byte{}, err
	}
	defer file.Close()

	// Parse the torrent file to extract the info dictionary
	var torrentMap map[string]interface{}
	if err := bencode.Unmarshal(file, &torrentMap); err != nil {
		return [20]byte{}, err
	}

	// Get the info dictionary
	info, ok := torrentMap["info"]
	if !ok {
		return [20]byte{}, fmt.Errorf("missing info dictionary")
	}

	// Re-encode the info dictionary to get its bencode representation
	var buf bytes.Buffer
	if err := bencode.Marshal(&buf, info); err != nil {
		return [20]byte{}, err
	}

	// Calculate SHA1 hash
	return sha1.Sum(buf.Bytes()), nil
}

// NumPieces returns the total number of pieces
func (t *TorrentMeta) NumPieces() int {
	return len(t.Pieces)
}

// PieceSize returns the size of a specific piece (last piece may be smaller)
func (t *TorrentMeta) PieceSize(index int) int {
	if index < 0 || index >= len(t.Pieces) {
		return 0
	}

	// Last piece may be smaller
	if index == len(t.Pieces)-1 {
		remainder := t.Length % t.PieceLength
		if remainder != 0 {
			return remainder
		}
	}

	return t.PieceLength
}
