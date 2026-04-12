package peer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/yourusername/torrent-client/internal/tracker"
)

const (
	protocolID = "BitTorrent protocol"

	// Message IDs
	msgChoke      = 0
	msgUnchoke    = 1
	msgInterested = 2
	msgBitfield   = 5
	msgRequest    = 6
	msgPiece      = 7

	dialTimeout      = 3 * time.Second
	handshakeTimeout = 5 * time.Second
	readTimeout      = 10 * time.Second
)

// Connection represents an active connection to a peer
type Connection struct {
	Conn net.Conn
	Peer tracker.Peer
}

// Message represents a BitTorrent protocol message
type Message struct {
	ID      byte
	Payload []byte
}

// ConnectAndHandshake connects to a peer over TCP and performs the BitTorrent
// handshake. Returns an error if the connection fails or the InfoHash doesn't match.
func ConnectAndHandshake(peer tracker.Peer, infoHash, peerID [20]byte) (*Connection, error) {
	conn, err := net.DialTimeout("tcp", peer.String(), dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", peer, err)
	}

	if err := performHandshake(conn, infoHash, peerID); err != nil {
		conn.Close()
		return nil, err
	}

	return &Connection{Conn: conn, Peer: peer}, nil
}

// performHandshake sends and receives the BitTorrent handshake, verifying InfoHash.
// Format: <1 byte pstrlen><19 bytes "BitTorrent protocol"><8 zero bytes><20 bytes InfoHash><20 bytes PeerID>
func performHandshake(conn net.Conn, infoHash, peerID [20]byte) error {
	conn.SetDeadline(time.Now().Add(handshakeTimeout))
	defer conn.SetDeadline(time.Time{})

	// Send
	buf := make([]byte, 68) // 1 + 19 + 8 + 20 + 20
	buf[0] = byte(len(protocolID))
	copy(buf[1:20], protocolID)
	copy(buf[28:48], infoHash[:])
	copy(buf[48:68], peerID[:])
	if _, err := conn.Write(buf); err != nil {
		return fmt.Errorf("handshake send: %w", err)
	}

	// Receive
	var pstrLen [1]byte
	if _, err := io.ReadFull(conn, pstrLen[:]); err != nil {
		return fmt.Errorf("handshake recv pstrlen: %w", err)
	}
	if int(pstrLen[0]) != len(protocolID) {
		return fmt.Errorf("unexpected protocol string length: %d", pstrLen[0])
	}

	resp := make([]byte, int(pstrLen[0])+48) // pstr + reserved + InfoHash + PeerID
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("handshake recv body: %w", err)
	}

	// Verify InfoHash (after pstr + 8 reserved bytes)
	plen := int(pstrLen[0])
	if !bytes.Equal(resp[plen+8:plen+28], infoHash[:]) {
		return fmt.Errorf("info hash mismatch")
	}

	return nil
}

// Close closes the peer connection
func (c *Connection) Close() error {
	return c.Conn.Close()
}

// ReadMessage reads one length-prefixed message from the connection.
// Returns nil for keep-alive messages (length == 0).
func ReadMessage(conn net.Conn) (*Message, error) {
	conn.SetDeadline(time.Now().Add(readTimeout))
	defer conn.SetDeadline(time.Time{})

	var lenBuf [4]byte
	if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lenBuf[:])
	if length == 0 {
		return nil, nil // keep-alive
	}

	var idBuf [1]byte
	if _, err := io.ReadFull(conn, idBuf[:]); err != nil {
		return nil, err
	}

	payload := make([]byte, length-1)
	if len(payload) > 0 {
		if _, err := io.ReadFull(conn, payload); err != nil {
			return nil, err
		}
	}

	return &Message{ID: idBuf[0], Payload: payload}, nil
}

// SendMessage writes a length-prefixed message to the connection
func SendMessage(conn net.Conn, msg *Message) error {
	length := uint32(1 + len(msg.Payload))
	buf := make([]byte, 4+length)
	binary.BigEndian.PutUint32(buf[0:4], length)
	buf[4] = msg.ID
	copy(buf[5:], msg.Payload)
	_, err := conn.Write(buf)
	return err
}

// SendInterested sends an Interested message
func SendInterested(conn net.Conn) error {
	return SendMessage(conn, &Message{ID: msgInterested})
}

// SendRequest sends a Request message for a 16KB block at (index, begin)
func SendRequest(conn net.Conn, index, begin, length uint32) error {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload[0:4], index)
	binary.BigEndian.PutUint32(payload[4:8], begin)
	binary.BigEndian.PutUint32(payload[8:12], length)
	return SendMessage(conn, &Message{ID: msgRequest, Payload: payload})
}
