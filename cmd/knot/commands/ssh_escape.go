package commands

import (
	"encoding/json"
	"fmt"
	"knot/internal/protocol"
	"strings"
)

type sshEscapeAction int

const (
	sshEscapeNone sshEscapeAction = iota
	sshEscapeSend
	sshEscapeBroadcast
	sshEscapeHelp
	sshEscapeLocalOutput
)

type sshEscapeResult struct {
	Action  sshEscapeAction
	Payload []byte
	Request protocol.BroadcastRequest
	Message string
}

type sshEscapeParser struct {
	escape      byte
	enabled     bool
	atLineStart bool
	pending     bool
	joining     bool
	joinGroup   []byte
}

func newSSHEscapeParser(value string) sshEscapeParser {
	if value == "" {
		value = "~"
	}
	if value == "none" {
		return sshEscapeParser{}
	}
	return sshEscapeParser{
		escape:      value[0],
		enabled:     true,
		atLineStart: true,
	}
}

func (p *sshEscapeParser) Process(payload []byte) []sshEscapeResult {
	if !p.enabled {
		return []sshEscapeResult{{Action: sshEscapeSend, Payload: payload}}
	}

	results := make([]sshEscapeResult, 0, 1)
	send := make([]byte, 0, len(payload))
	flushSend := func() {
		if len(send) == 0 {
			return
		}
		out := make([]byte, len(send))
		copy(out, send)
		results = append(results, sshEscapeResult{Action: sshEscapeSend, Payload: out})
		send = send[:0]
	}

	for i := 0; i < len(payload); i++ {
		b := payload[i]
		if p.joining {
			results = append(results, p.processJoinInput(b)...)
			continue
		}

		if p.pending {
			switch b {
			case p.escape:
				send = append(send, p.escape)
				p.updateLineStart(p.escape)
			case 'B':
				flushSend()
				results = append(results, sshEscapeResult{
					Action:  sshEscapeBroadcast,
					Request: protocol.BroadcastRequest{Action: "leave"},
				})
				p.afterLocalCommand()
			case 'p':
				flushSend()
				results = append(results, sshEscapeResult{
					Action:  sshEscapeBroadcast,
					Request: protocol.BroadcastRequest{Action: "pause"},
				})
				p.afterLocalCommand()
			case 'r':
				flushSend()
				results = append(results, sshEscapeResult{
					Action:  sshEscapeBroadcast,
					Request: protocol.BroadcastRequest{Action: "resume"},
				})
				p.afterLocalCommand()
			case 'j', 'J':
				flushSend()
				group, consumed, hasLineTerminator := readEscapeArgument(payload[i+1:])
				if group == "" {
					p.pending = false
					p.atLineStart = true
					p.startJoinPrompt()
					results = append(results, sshEscapeResult{Action: sshEscapeLocalOutput, Payload: []byte("\r\n[knot] broadcast group: ")})
					next := i + 1 + consumed
					if hasLineTerminator {
						next++
					}
					return append(results, p.Process(payload[next:])...)
				}
				results = append(results, sshEscapeResult{
					Action:  sshEscapeBroadcast,
					Request: protocol.BroadcastRequest{Action: "join", Group: group},
				})
				p.pending = false
				p.afterLocalCommand()
				return append(results, p.Process(payload[i+1+consumed:])...)
			case '?':
				flushSend()
				results = append(results, sshEscapeResult{Action: sshEscapeHelp, Message: sshEscapeHelpTextFor(string([]byte{p.escape}))})
				p.afterLocalCommand()
			default:
				send = append(send, p.escape, b)
				if isLineInterrupt(b) {
					p.atLineStart = true
					break
				}
				p.updateLineStart(b)
			}
			p.pending = false
			continue
		}

		if p.atLineStart && b == p.escape {
			p.pending = true
			continue
		}
		send = append(send, b)
		if isLineInterrupt(b) {
			p.atLineStart = true
			continue
		}
		p.updateLineStart(b)
	}
	flushSend()
	return results
}

func (p *sshEscapeParser) Flush() []sshEscapeResult {
	if !p.pending {
		return nil
	}
	p.pending = false
	p.atLineStart = false
	return []sshEscapeResult{{Action: sshEscapeSend, Payload: []byte{p.escape}}}
}

func (p *sshEscapeParser) updateLineStart(b byte) {
	p.atLineStart = b == '\n' || b == '\r'
}

func isLineInterrupt(b byte) bool {
	return b == 0x03 || b == 0x04 || b == 0x1a
}

func (p *sshEscapeParser) afterLocalCommand() {
	p.atLineStart = true
}

func (p *sshEscapeParser) startJoinPrompt() {
	p.joining = true
	p.joinGroup = p.joinGroup[:0]
}

func (p *sshEscapeParser) processJoinInput(b byte) []sshEscapeResult {
	switch b {
	case '\r', '\n':
		group := strings.TrimSpace(string(p.joinGroup))
		p.joining = false
		p.joinGroup = p.joinGroup[:0]
		p.afterLocalCommand()
		results := []sshEscapeResult{{Action: sshEscapeLocalOutput, Payload: []byte("\r\n")}}
		if group == "" {
			return append(results, sshEscapeResult{Action: sshEscapeHelp, Message: fmt.Sprintf("[broadcast escapes: usage %sj <group>]", string([]byte{p.escape}))})
		}
		return append(results, sshEscapeResult{
			Action:  sshEscapeBroadcast,
			Request: protocol.BroadcastRequest{Action: "join", Group: group},
		})
	case 0x03:
		p.joining = false
		p.joinGroup = p.joinGroup[:0]
		p.afterLocalCommand()
		return []sshEscapeResult{{Action: sshEscapeLocalOutput, Payload: []byte("^C\r\n")}}
	case 0x7f, 0x08:
		if len(p.joinGroup) == 0 {
			return nil
		}
		p.joinGroup = p.joinGroup[:len(p.joinGroup)-1]
		return []sshEscapeResult{{Action: sshEscapeLocalOutput, Payload: []byte("\b \b")}}
	case 0x15:
		if len(p.joinGroup) == 0 {
			return nil
		}
		clear := strings.Repeat("\b \b", len(p.joinGroup))
		p.joinGroup = p.joinGroup[:0]
		return []sshEscapeResult{{Action: sshEscapeLocalOutput, Payload: []byte(clear)}}
	default:
		if b < 0x20 || b == 0x7f {
			return nil
		}
		p.joinGroup = append(p.joinGroup, b)
		return []sshEscapeResult{{Action: sshEscapeLocalOutput, Payload: []byte{b}}}
	}
}

func sshEscapeHelpText() string {
	return sshEscapeHelpTextFor("~")
}

func sshEscapeHelpTextFor(value string) string {
	parser := newSSHEscapeParser(value)
	if !parser.enabled {
		return "[broadcast escapes disabled]"
	}
	escape := string([]byte{parser.escape})
	return fmt.Sprintf("[broadcast escapes: %sj <group> join, %sB leave, %sp pause, %sr resume, %s? help, %s%s send %s]",
		escape, escape, escape, escape, escape, escape, escape, escape)
}

func readEscapeArgument(payload []byte) (string, int, bool) {
	i := 0
	for i < len(payload) && (payload[i] == ' ' || payload[i] == '\t') {
		i++
	}
	start := i
	for i < len(payload) && payload[i] != '\r' && payload[i] != '\n' {
		i++
	}
	return strings.TrimSpace(string(payload[start:i])), i, i < len(payload)
}

func marshalBroadcastRequest(req protocol.BroadcastRequest) ([]byte, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal broadcast request: %w", err)
	}
	return payload, nil
}
