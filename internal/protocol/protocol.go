package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Magic bytes for the Knot protocol: "KN" (0x4B, 0x4E)
var Magic = [2]byte{0x4B, 0x4E}

const (
	Version1 uint8 = 0x01
)

const (
	TypeReq    uint8 = 0x01
	TypeResp   uint8 = 0x02
	TypeData   uint8 = 0x03
	TypeSignal uint8 = 0x04
)

const MaxPayloadSize = 10 * 1024 * 1024 // 10MB

// Header represents the Knot protocol header.
// Structure: Magic(2) | Version(1) | Type(1) | Reserved(1) | PayloadLength(3)
// Total Header Size: 8 bytes
type Header struct {
	Magic    [2]byte
	Version  uint8
	Type     uint8
	Reserved uint8
	Length   uint32 // Using 4 bytes for length for simplicity, but we'll limit it.
}

const HeaderSize = 8

// Encode serializes the header into a byte slice.
func (h *Header) Encode() []byte {
	buf := make([]byte, HeaderSize)
	copy(buf[0:2], h.Magic[:])
	buf[2] = h.Version
	buf[3] = h.Type
	buf[4] = h.Reserved
	// Using the last 3 bytes for length would be more complex, 
	// let's stick to 4 bytes and just ensure we stay within 8 bytes total.
	// Redefining Header structure for clarity:
	// Magic(2) | Version(1) | Type(1) | Length(4)
	binary.BigEndian.PutUint32(buf[4:8], h.Length)
	return buf
}

// DecodeHeader deserializes the header from a reader.
func DecodeHeader(r io.Reader) (*Header, error) {
	buf := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	if buf[0] != Magic[0] || buf[1] != Magic[1] {
		return nil, fmt.Errorf("invalid magic bytes: 0x%02X 0x%02X", buf[0], buf[1])
	}

	h := &Header{
		Magic:   [2]byte{buf[0], buf[1]},
		Version: buf[2],
		Type:    buf[3],
		// buf[4:8] is length
		Length: binary.BigEndian.Uint32(buf[4:8]),
	}
	return h, nil
}

// Message represents a full protocol message.
type Message struct {
	Header  Header
	Payload []byte
}

// ReadMessage reads a full message from a reader.
func ReadMessage(r io.Reader) (*Message, error) {
	header, err := DecodeHeader(r)
	if err != nil {
		return nil, err
	}

	if header.Length > MaxPayloadSize {
		return nil, fmt.Errorf("payload too large: %d > %d", header.Length, MaxPayloadSize)
	}

	payload := make([]byte, header.Length)
	if header.Length > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}
	}

	return &Message{
		Header:  *header,
		Payload: payload,
	}, nil
}

// WriteMessage writes a full message to a writer.
func WriteMessage(w io.Writer, msgType uint8, payload []byte) error {
	if len(payload) > MaxPayloadSize {
		return fmt.Errorf("payload too large to write: %d > %d", len(payload), MaxPayloadSize)
	}

	header := Header{
		Magic:   Magic,
		Version: Version1,
		Type:    msgType,
		Length:  uint32(len(payload)),
	}

	if _, err := w.Write(header.Encode()); err != nil {
		return err
	}

	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}

	return nil
}
