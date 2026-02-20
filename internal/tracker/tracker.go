package tracker

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/jackpal/bencode-go"
	"github.com/yourusername/torrent-client/internal/torrent"
)

// Peer represents a peer in the BitTorrent network
type Peer struct {
	IP   string
	Port uint16
}

// trackerResponse represents the response from the tracker
type trackerResponse struct {
	Interval int    `bencode:"interval"`
	Peers    string `bencode:"peers"` // Compact format: 6 bytes per peer (4 IP + 2 port)
}

const (
	// Port we're listening on (not actually listening, but required by tracker)
	peerPort = 6881
)

// GetPeers contacts the tracker and retrieves a list of peers
func GetPeers(meta *torrent.TorrentMeta) ([]Peer, error) {
	// Generate a unique peer ID (20 bytes)
	peerID, err := generatePeerID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate peer ID: %w", err)
	}

	// Build tracker request URL
	trackerURL, err := buildTrackerURL(meta, peerID)
	if err != nil {
		return nil, fmt.Errorf("failed to build tracker URL: %w", err)
	}

	// Make HTTP GET request to tracker
	resp, err := http.Get(trackerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to contact tracker: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tracker returned status: %s", resp.Status)
	}

	// Parse tracker response
	var trackerResp trackerResponse
	if err := bencode.Unmarshal(resp.Body, &trackerResp); err != nil {
		return nil, fmt.Errorf("failed to decode tracker response: %w", err)
	}

	// Parse peers from compact format
	peers, err := parsePeers(trackerResp.Peers)
	if err != nil {
		return nil, fmt.Errorf("failed to parse peers: %w", err)
	}

	return peers, nil
}

// buildTrackerURL constructs the tracker announce URL with all required parameters
func buildTrackerURL(meta *torrent.TorrentMeta, peerID string) (string, error) {
	baseURL, err := url.Parse(meta.Announce)
	if err != nil {
		return "", err
	}

	// Calculate bytes left to download
	left := meta.Length

	// Build query parameters
	params := url.Values{
		"info_hash":  []string{string(meta.InfoHash[:])},
		"peer_id":    []string{peerID},
		"port":       []string{strconv.Itoa(peerPort)},
		"uploaded":   []string{"0"},
		"downloaded": []string{"0"},
		"left":       []string{strconv.Itoa(left)},
		"compact":    []string{"1"}, // Request compact peer format
	}

	baseURL.RawQuery = params.Encode()
	return baseURL.String(), nil
}

// generatePeerID generates a random 20-byte peer ID
// Format: -GO0001-<12 random bytes>
func generatePeerID() (string, error) {
	// Use a prefix to identify our client
	prefix := "-GO0001-"

	// Generate 12 random bytes
	randomBytes := make([]byte, 12)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	return prefix + string(randomBytes), nil
}

// parsePeers decodes peers from compact format (6 bytes per peer)
// Format: 4 bytes IP + 2 bytes port (big-endian)
func parsePeers(peersData string) ([]Peer, error) {
	const peerSize = 6 // 4 bytes IP + 2 bytes port

	if len(peersData)%peerSize != 0 {
		return nil, fmt.Errorf("invalid peers data length: %d (not divisible by %d)", len(peersData), peerSize)
	}

	numPeers := len(peersData) / peerSize
	peers := make([]Peer, numPeers)

	for i := 0; i < numPeers; i++ {
		offset := i * peerSize

		// Extract IP (4 bytes)
		ip := peersData[offset : offset+4]
		ipStr := fmt.Sprintf("%d.%d.%d.%d",
			uint8(ip[0]),
			uint8(ip[1]),
			uint8(ip[2]),
			uint8(ip[3]),
		)

		// Extract port (2 bytes, big-endian)
		port := binary.BigEndian.Uint16([]byte(peersData[offset+4 : offset+6]))

		peers[i] = Peer{
			IP:   ipStr,
			Port: port,
		}
	}

	return peers, nil
}

// String returns a string representation of the peer
func (p Peer) String() string {
	return fmt.Sprintf("%s:%d", p.IP, p.Port)
}
