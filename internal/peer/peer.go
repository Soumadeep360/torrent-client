package peer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/yourusername/torrent-client/internal/tracker"
)

const (
	// Protocol identifier for BitTorrent
	protocolID = "BitTorrent protocol"

	// Message types
	msgChoke         = 0
	msgUnchoke       = 1
	msgInterested    = 2
	msgNotInterested = 3
	msgHave          = 4
	msgBitfield      = 5
	msgRequest       = 6
	msgPiece         = 7
	msgCancel        = 8

	// Timeouts
	dialTimeout      = 3 * time.Second
	handshakeTimeout = 5 * time.Second
	readTimeout      = 10 * time.Second
)

// Connection represents a connection to a peer
type Connection struct {
	Conn     net.Conn // Exported for access by downloader
	Peer     tracker.Peer
	Choked   bool
	Bitfield []byte
	mu       sync.Mutex
}

// Handshake represents the BitTorrent handshake message
type Handshake struct {
	Pstr     string   // Protocol string
	InfoHash [20]byte // Info hash from torrent
	PeerID   [20]byte // Our peer ID
}

// Message represents a BitTorrent protocol message
type Message struct {
	ID      byte
	Payload []byte
}

// ConnectToPeer establishes a TCP connection to a peer
func ConnectToPeer(peer tracker.Peer, timeout time.Duration) (net.Conn, error) {
	address := peer.String()

	// Set dial timeout
	if timeout == 0 {
		timeout = dialTimeout
	}

	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", address, err)
	}

	return conn, nil
}

// PerformHandshake performs the BitTorrent handshake with a peer
func PerformHandshake(conn net.Conn, infoHash, peerID [20]byte) (*Handshake, error) {
	// Set deadline for handshake
	conn.SetDeadline(time.Now().Add(handshakeTimeout))
	defer conn.SetDeadline(time.Time{}) // Reset deadline

	// Send handshake
	if err := sendHandshake(conn, infoHash, peerID); err != nil {
		return nil, fmt.Errorf("failed to send handshake: %w", err)
	}

	// Receive handshake
	hs, err := receiveHandshake(conn)
	if err != nil {
		return nil, fmt.Errorf("failed to receive handshake: %w", err)
	}

	// Verify info hash matches
	if !bytes.Equal(hs.InfoHash[:], infoHash[:]) {
		return nil, fmt.Errorf("info hash mismatch")
	}

	return hs, nil
}

// sendHandshake sends a handshake message to the peer
// Format: <pstrlen><pstr><reserved><info_hash><peer_id>
func sendHandshake(conn net.Conn, infoHash, peerID [20]byte) error {
	buf := make([]byte, 68) // 1 + 19 + 8 + 20 + 20

	// Protocol string length
	buf[0] = byte(len(protocolID))

	// Protocol string
	copy(buf[1:20], protocolID)

	// Reserved bytes (8 bytes of zeros)
	// Already zero from make()

	// Info hash
	copy(buf[28:48], infoHash[:])

	// Peer ID
	copy(buf[48:68], peerID[:])

	_, err := conn.Write(buf)
	return err
}

// receiveHandshake receives and parses a handshake message from the peer
func receiveHandshake(conn net.Conn) (*Handshake, error) {
	// Read protocol string length
	lengthBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, lengthBuf); err != nil {
		return nil, err
	}
	pstrLen := int(lengthBuf[0])

	if pstrLen != len(protocolID) {
		return nil, fmt.Errorf("invalid protocol string length: %d", pstrLen)
	}

	// Read rest of handshake: pstr + reserved + info_hash + peer_id
	buf := make([]byte, pstrLen+48) // 19 + 8 + 20 + 20
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, err
	}

	hs := &Handshake{
		Pstr: string(buf[0:pstrLen]),
	}

	// Extract info hash (after pstr + reserved)
	copy(hs.InfoHash[:], buf[pstrLen+8:pstrLen+28])

	// Extract peer ID
	copy(hs.PeerID[:], buf[pstrLen+28:pstrLen+48])

	return hs, nil
}

// ReadMessage reads a message from the peer connection
// Format: <length><id><payload>
func ReadMessage(conn net.Conn) (*Message, error) {
	// Set read deadline
	conn.SetDeadline(time.Now().Add(readTimeout))
	defer conn.SetDeadline(time.Time{})

	// Read message length (4 bytes)
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lengthBuf); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lengthBuf)

	// Keep-alive message (length = 0)
	if length == 0 {
		return nil, nil
	}

	// Read message ID (1 byte)
	idBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, idBuf); err != nil {
		return nil, err
	}

	// Read payload (length - 1 bytes)
	payloadLen := length - 1
	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(conn, payload); err != nil {
			return nil, err
		}
	}

	return &Message{
		ID:      idBuf[0],
		Payload: payload,
	}, nil
}

// SendMessage sends a message to the peer
func SendMessage(conn net.Conn, msg *Message) error {
	// Message length = 1 (ID) + len(payload)
	length := uint32(1 + len(msg.Payload))

	// Build message: <length><id><payload>
	buf := make([]byte, 4+length)
	binary.BigEndian.PutUint32(buf[0:4], length)
	buf[4] = msg.ID
	copy(buf[5:], msg.Payload)

	_, err := conn.Write(buf)
	return err
}

// SendInterested sends an interested message to the peer
func SendInterested(conn net.Conn) error {
	return SendMessage(conn, &Message{ID: msgInterested})
}

// SendNotInterested sends a not interested message to the peer
func SendNotInterested(conn net.Conn) error {
	return SendMessage(conn, &Message{ID: msgNotInterested})
}

// SendRequest sends a piece request to the peer
// Payload: <index><begin><length>
func SendRequest(conn net.Conn, index, begin, length uint32) error {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload[0:4], index)
	binary.BigEndian.PutUint32(payload[4:8], begin)
	binary.BigEndian.PutUint32(payload[8:12], length)

	return SendMessage(conn, &Message{
		ID:      msgRequest,
		Payload: payload,
	})
}

// ConnectAndHandshake is a helper that connects to a peer and performs handshake
func ConnectAndHandshake(peer tracker.Peer, infoHash, peerID [20]byte) (*Connection, error) {
	conn, err := ConnectToPeer(peer, dialTimeout)
	if err != nil {
		return nil, err
	}

	// Perform handshake
	_, err = PerformHandshake(conn, infoHash, peerID)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &Connection{
		Conn:   conn,
		Peer:   peer,
		Choked: true, // Peers start choked
	}, nil
}

// Close closes the peer connection
func (c *Connection) Close() error {
	return c.Conn.Close()
}

// String returns a string representation of the connection
func (c *Connection) String() string {
	return c.Peer.String()
}
