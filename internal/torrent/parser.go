package torrent

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"strconv"

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

// calculateInfoHash computes the SHA1 hash of the raw bencoded info dictionary.
// We extract the info dict bytes directly from the file instead of unmarshaling
// into map[string]interface{}, because jackpal/bencode-go cannot set values in
// map elements (they are not addressable in reflect).
func calculateInfoHash(path string) ([20]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return [20]byte{}, err
	}
	infoBytes, err := extractInfoDictBytes(data)
	if err != nil {
		return [20]byte{}, err
	}
	return sha1.Sum(infoBytes), nil
}

// extractInfoDictBytes finds the "info" key in the top-level bencode dict and
// returns the raw bencode bytes of its value (the info dictionary).
func extractInfoDictBytes(data []byte) ([]byte, error) {
	r := bytes.NewReader(data)
	// Root must be a dict
	b, err := r.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("read root: %w", err)
	}
	if b != 'd' {
		return nil, fmt.Errorf("root is not a dict (got %c)", b)
	}
	// Read key-value pairs until we find "info"
	for {
		key, err := readBencodeString(r)
		if err != nil {
			if err == io.EOF {
				return nil, fmt.Errorf("missing info dictionary")
			}
			return nil, err
		}
		if key == "info" {
			// Next value is the info dict; its first byte must be 'd'
			next, err := r.ReadByte()
			if err != nil {
				return nil, fmt.Errorf("read info dict: %w", err)
			}
			if next != 'd' {
				return nil, fmt.Errorf("info value is not a dict (got %c)", next)
			}
			// Capture from 'd' to matching 'e' (inclusive)
			start := int(r.Size()) - r.Len()
			infoStart := start - 1 // include the 'd' we just read
			end, err := findMatchingEnd(data[infoStart:])
			if err != nil {
				return nil, err
			}
			return data[infoStart : infoStart+end], nil
		}
		// Skip this key's value so we can read the next key
		if err := skipBencodeValue(r); err != nil {
			return nil, err
		}
	}
}

// readBencodeString reads a bencode string: <length>:<data>
func readBencodeString(r *bytes.Reader) (string, error) {
	var lenBuf []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == ':' {
			break
		}
		if b < '0' || b > '9' {
			return "", fmt.Errorf("invalid string length character %c", b)
		}
		lenBuf = append(lenBuf, b)
	}
	if len(lenBuf) == 0 {
		return "", fmt.Errorf("empty string length")
	}
	length, err := strconv.Atoi(string(lenBuf))
	if err != nil || length < 0 {
		return "", fmt.Errorf("invalid string length %s", string(lenBuf))
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

// skipBencodeValue advances r past one bencode value (string, int, list, or dict).
func skipBencodeValue(r *bytes.Reader) error {
	b, err := r.ReadByte()
	if err != nil {
		return err
	}
	switch b {
	case 'i':
		// integer: i<digits>e
		for {
			c, err := r.ReadByte()
			if err != nil {
				return err
			}
			if c == 'e' {
				return nil
			}
		}
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		// string: <length>:<data>
		lenBuf := []byte{b}
		for {
			c, err := r.ReadByte()
			if err != nil {
				return err
			}
			if c == ':' {
				break
			}
			if c < '0' || c > '9' {
				return fmt.Errorf("invalid string length")
			}
			lenBuf = append(lenBuf, c)
		}
		length, err := strconv.Atoi(string(lenBuf))
		if err != nil {
			return err
		}
		_, err = r.Seek(int64(length), io.SeekCurrent)
		return err
	case 'l':
		// list: skip elements until 'e'
		for {
			next, err := r.ReadByte()
			if err != nil {
				return err
			}
			if next == 'e' {
				return nil
			}
			if err := r.UnreadByte(); err != nil {
				return err
			}
			if err := skipBencodeValue(r); err != nil {
				return err
			}
		}
	case 'd':
		// dict: skip key-value pairs until 'e'
		for {
			next, err := r.ReadByte()
			if err != nil {
				return err
			}
			if next == 'e' {
				return nil
			}
			if err := r.UnreadByte(); err != nil {
				return err
			}
			if err := skipBencodeValue(r); err != nil { // key (string)
				return err
			}
			if err := skipBencodeValue(r); err != nil { // value
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected bencode type %c", b)
	}
}

// findMatchingEnd returns the length of the bencode dict starting at data[0].
// So the dict is data[0:returned]. Skips string contents so 'd'/'e' in "pieces" are not counted.
func findMatchingEnd(data []byte) (int, error) {
	if len(data) == 0 || data[0] != 'd' {
		return 0, fmt.Errorf("expected dict start")
	}
	i := 1 // after 'd'
	for i < len(data) {
		if data[i] == 'e' {
			return i + 1, nil
		}
		// key (must be string)
		n, err := bencodeValueLength(data[i:])
		if err != nil {
			return 0, err
		}
		i += n
		if i >= len(data) {
			return 0, fmt.Errorf("truncated at key end")
		}
		// value
		n, err = bencodeValueLength(data[i:])
		if err != nil {
			return 0, err
		}
		i += n
	}
	return 0, fmt.Errorf("no matching end for dict")
}

// bencodeValueLength returns the length in bytes of one bencode value at data[0].
func bencodeValueLength(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, fmt.Errorf("empty data")
	}
	switch data[0] {
	case 'i':
		// i<digits>e
		end := bytes.IndexByte(data, 'e')
		if end == -1 {
			return 0, fmt.Errorf("invalid integer")
		}
		return end + 1, nil
	case 'd', 'l':
		// Skip key-value pairs (dict) or elements (list) until matching 'e'
		i := 1
		for i < len(data) {
			if data[i] == 'e' {
				return i + 1, nil
			}
			if data[0] == 'd' {
				// Dict: key (string) then value
				n, err := bencodeValueLength(data[i:])
				if err != nil {
					return 0, err
				}
				i += n
			}
			// Value (dict value, or list element)
			n, err := bencodeValueLength(data[i:])
			if err != nil {
				return 0, err
			}
			i += n
		}
		return 0, fmt.Errorf("no matching end for dict/list")
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return bencodeStringLength(data)
	default:
		return 0, fmt.Errorf("unexpected bencode type %c", data[0])
	}
}

func bencodeStringLength(data []byte) (int, error) {
	colon := bytes.IndexByte(data, ':')
	if colon <= 0 {
		return 0, fmt.Errorf("invalid string length")
	}
	length, err := strconv.Atoi(string(data[:colon]))
	if err != nil || length < 0 {
		return 0, fmt.Errorf("invalid string length")
	}
	return colon + 1 + length, nil
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
