package protocol

import (
	"fmt"
	"io"
	"sync"
)

// Magic bytes for the Knot protocol: "KN" (0x4B, 0x4E)
var Magic = [2]byte{0x4B, 0x4E}

const (
	Version1 uint8 = 0x01
)

var (
	// Default buffer size for messages (32KB + HeaderSize)
	defaultBufferSize = 32*1024 + HeaderSize
	msgPool           = sync.Pool{
		New: func() interface{} {
			return make([]byte, defaultBufferSize)
		},
	}
)

const (
	TypeReq    uint8 = 0x01
	TypeResp   uint8 = 0x02
	TypeData   uint8 = 0x03
	TypeSignal uint8 = 0x04
	TypeHostKeyConfirm uint8 = 0x05
	TypeSFTPReq uint8 = 0x06
	TypeSessionListReq uint8 = 0x07
	TypeCWDUpdate uint8 = 0x08
	TypeDisconnect uint8 = 0x09
	TypeStatusReq uint8 = 0x0A
	TypeStatusResp uint8 = 0x0B
	TypeForwardReq uint8 = 0x0C
	TypeForwardListReq uint8 = 0x0D
	TypeForwardListResp uint8 = 0x0E
	TypeForwardNotify   uint8 = 0x0F
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
	Alias        string `json:"alias"`
	Term         string `json:"term"`
	Rows         int    `json:"rows"`
	Cols         int    `json:"cols"`
	ForwardAgent bool   `json:"forward_agent"`
	SSHAuthSock  string `json:"ssh_auth_sock,omitempty"`
}

// ResizePayload defines the payload for a terminal resize signal.
type ResizePayload struct {
	Rows int `json:"rows"`
	Cols int `json:"cols"`
}

// StatusResponse defines the payload for a status response message.
type StatusResponse struct {
	DaemonPID      int             `json:"daemon_pid"`
	Uptime         string          `json:"uptime"`
	UDSPath        string          `json:"uds_path"`
	MemoryUsage    uint64          `json:"memory_usage_bytes"`
	PoolStats      []PoolEntryStat `json:"pool_stats"`
	ActiveSessions int             `json:"active_sessions"`
}

// PoolEntryStat defines the statistics for a single SSH pool entry.
type PoolEntryStat struct {
	Alias    string `json:"alias"`
	Host     string `json:"host"`
	IdleTime string `json:"idle_time"`
	RefCount int    `json:"ref_count"`
}

// ForwardProtocolConfig defines the configuration for a single port forward.
type ForwardProtocolConfig struct {
	Type       string `json:"type"`
	LocalPort  int    `json:"local_port"`
	RemoteAddr string `json:"remote_addr,omitempty"`
	Enabled    bool   `json:"enabled"`
}

// ForwardRequest defines the payload for a port forwarding management request.
type ForwardRequest struct {
	Action string                `json:"action"` // add, remove, enable, disable
	Alias  string                `json:"alias"`
	Config ForwardProtocolConfig `json:"config"`
	IsTemp bool                  `json:"is_temp"`
}

// ForwardStatus defines the status of a single port forward.
type ForwardStatus struct {
	Alias      string `json:"alias"`
	Type       string `json:"type"`
	LocalPort  int    `json:"local_port"`
	RemoteAddr string `json:"remote_addr"`
	Enabled    bool   `json:"enabled"`
	IsTemp     bool   `json:"is_temp"`
	Status     string `json:"status"` // active, inactive, error
	Error      string `json:"error,omitempty"`
}

// ForwardListResponse defines the payload for a port forwarding list response.
type ForwardListResponse struct {
	Alias    string          `json:"alias"`
	Forwards []ForwardStatus `json:"forwards"`
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

// EncodeTo serializes the header into an existing byte slice.
func (h *Header) EncodeTo(buf []byte) {
	copy(buf[0:2], h.Magic[:])
	buf[2] = h.Version
	buf[3] = h.Type
	buf[4] = h.Reserved
	// Using 3 bytes for length: buf[5:8]
	buf[5] = byte(h.Length >> 16)
	buf[6] = byte(h.Length >> 8)
	buf[7] = byte(h.Length)
}

// Encode serializes the header into a byte slice.
func (h *Header) Encode() []byte {
	buf := make([]byte, HeaderSize)
	h.EncodeTo(buf)
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
// It combines header and payload into a single write to ensure atomicity and reduce system calls.
func WriteMessage(w io.Writer, msgType uint8, reserved uint8, payload []byte) error {
	payloadLen := len(payload)
	if payloadLen > MaxPayloadSize {
		return fmt.Errorf("payload too large to write: %d > %d", payloadLen, MaxPayloadSize)
	}

	header := Header{
		Magic:    Magic,
		Version:  Version1,
		Type:     msgType,
		Reserved: reserved,
		Length:   uint32(payloadLen),
	}

	totalLen := HeaderSize + payloadLen
	var fullMsg []byte
	if totalLen <= defaultBufferSize {
		fullMsg = msgPool.Get().([]byte)
		defer msgPool.Put(fullMsg)
		fullMsg = fullMsg[:totalLen]
	} else {
		fullMsg = make([]byte, totalLen)
	}

	header.EncodeTo(fullMsg[0:HeaderSize])
	if payloadLen > 0 {
		copy(fullMsg[HeaderSize:], payload)
	}

	_, err := w.Write(fullMsg)
	return err
}
