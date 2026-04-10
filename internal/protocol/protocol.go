package protocol

import (
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

// SubTypes for TypeData (using Reserved field)
const (
	DataStdin  uint8 = 0x01
	DataStdout uint8 = 0x02
	DataStderr uint8 = 0x03
)

// SignalResize signals a terminal window resize.
const (
	SignalStop   uint8 = 0x01
	SignalResize uint8 = 0x02
)

// SSHRequest defines the payload for an SSH session request.
type SSHRequest struct {
	Alias string `json:"alias"`
	Term  string `json:"term"`
	Rows  int    `json:"rows"`
	Cols  int    `json:"cols"`
}

// ResizePayload defines the payload for a terminal resize signal.
type ResizePayload struct {
	Rows int `json:"rows"`
	Cols int `json:"cols"`
}

const MaxPayloadSize = 10 * 1024 * 1024 // 10MB

// Header represents the Knot protocol header.
// Structure: Magic(2) | Version(1) | Type(1) | Subtype(1) | Length(3)
// Total Header Size: 8 bytes
type Header struct {
	Magic    [2]byte
	Version  uint8
	Type     uint8
	Reserved uint8 // Now correctly mapped as Subtype/Reserved
	Length   uint32 // Using 3 bytes for length in wire format
}

const HeaderSize = 8

// Encode serializes the header into a byte slice.
func (h *Header) Encode() []byte {
	buf := make([]byte, HeaderSize)
	copy(buf[0:2], h.Magic[:])
	buf[2] = h.Version
	buf[3] = h.Type
	buf[4] = h.Reserved
	// Using 3 bytes for length: buf[5:8]
	buf[5] = byte(h.Length >> 16)
	buf[6] = byte(h.Length >> 8)
	buf[7] = byte(h.Length)
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

	version := buf[2]
	if version != Version1 {
		return nil, fmt.Errorf("unsupported protocol version: %d", version)
	}

	h := &Header{
		Magic:    [2]byte{buf[0], buf[1]},
		Version:  version,
		Type:     buf[3],
		Reserved: buf[4],
		// buf[5:8] is length (24-bit)
		Length: uint32(buf[5])<<16 | uint32(buf[6])<<8 | uint32(buf[7]),
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

// WriteMessage writes a full message to a writer with a reserved byte for sub-types.
func WriteMessage(w io.Writer, msgType uint8, reserved uint8, payload []byte) error {
	if len(payload) > MaxPayloadSize {
		return fmt.Errorf("payload too large to write: %d > %d", len(payload), MaxPayloadSize)
	}

	header := Header{
		Magic:    Magic,
		Version:  Version1,
		Type:     msgType,
		Reserved: reserved,
		Length:   uint32(len(payload)),
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
