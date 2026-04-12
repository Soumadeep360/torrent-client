package torrent

import (
	"crypto/sha1"
	"fmt"
	"os"

	"github.com/zeebo/bencode"
)

// TorrentMeta represents the metadata extracted from a .torrent file
type TorrentMeta struct {
	Announce    string   // Tracker URL
	Name        string   // File or directory name
	Length      int      // Total length in bytes (single file mode)
	PieceLength int      // Length of each piece in bytes
	Pieces      [][]byte // List of SHA1 hashes for each piece
	InfoHash    [20]byte // SHA1 hash of the info dictionary
}

// bencodeTorrent mirrors the top-level .torrent structure.
// Info is captured as a RawMessage so we can SHA1 the original bytes
// directly — re-encoding would change key order and produce a wrong hash.
type bencodeTorrent struct {
	Announce string             `bencode:"announce"`
	Info     bencode.RawMessage `bencode:"info"`
}

// bencodeInfo holds the structured fields inside the info dict
type bencodeInfo struct {
	Name        string `bencode:"name"`
	Length      int    `bencode:"length"`
	PieceLength int    `bencode:"piece length"`
	Pieces      string `bencode:"pieces"` // concatenated 20-byte SHA1 hashes
}

// ParseTorrentFile reads the .torrent file once and extracts all metadata.
func ParseTorrentFile(path string) (*TorrentMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read torrent file: %w", err)
	}

	// Pass 1: decode top-level dict, capturing info as raw bytes
	var bt bencodeTorrent
	if err := bencode.DecodeBytes(data, &bt); err != nil {
		return nil, fmt.Errorf("failed to decode bencode: %w", err)
	}

	// Pass 2: decode structured fields from the raw info bytes
	var info bencodeInfo
	if err := bencode.DecodeBytes(bt.Info, &info); err != nil {
		return nil, fmt.Errorf("failed to decode info dict: %w", err)
	}

	// Pass 3: split the pieces string into individual 20-byte hashes
	pieces, err := splitPieces(info.Pieces)
	if err != nil {
		return nil, fmt.Errorf("failed to split pieces: %w", err)
	}

	return &TorrentMeta{
		Announce:    bt.Announce,
		Name:        info.Name,
		Length:      info.Length,
		PieceLength: info.PieceLength,
		Pieces:      pieces,
		InfoHash:    sha1.Sum(bt.Info), // SHA1 of the original raw bytes
	}, nil
}

// splitPieces splits the concatenated piece hashes into individual 20-byte slices
func splitPieces(piecesStr string) ([][]byte, error) {
	const hashLen = 20

	if len(piecesStr)%hashLen != 0 {
		return nil, fmt.Errorf("invalid pieces length: %d (not divisible by %d)", len(piecesStr), hashLen)
	}

	numPieces := len(piecesStr) / hashLen
	pieces := make([][]byte, numPieces)
	for i := 0; i < numPieces; i++ {
		pieces[i] = []byte(piecesStr[i*hashLen : (i+1)*hashLen])
	}
	return pieces, nil
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
	if index == len(t.Pieces)-1 {
		if remainder := t.Length % t.PieceLength; remainder != 0 {
			return remainder
		}
	}
	return t.PieceLength
}
