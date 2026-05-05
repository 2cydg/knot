//go:build windows

package sftp

import (
	"syscall"
	"unsafe"

	"github.com/chzyer/readline"
)

const (
	windowsEventKey = 0x0001

	windowsVKControl  = 0x11
	windowsVKMenu     = 0x12
	windowsVKLeft     = 0x25
	windowsVKUp       = 0x26
	windowsVKRight    = 0x27
	windowsVKDown     = 0x28
	windowsVKDelete   = 0x2E
	windowsVKHome     = 0x24
	windowsVKEnd      = 0x23
	windowsVKLControl = 0xA2
	windowsVKRControl = 0xA3
)

type windowsInputRecord struct {
	EventType uint16
	Padding   uint16
	Event     [16]byte
}

type windowsKeyEventRecord struct {
	KeyDown         int32
	RepeatCount     uint16
	VirtualKeyCode  uint16
	VirtualScanCode uint16
	UnicodeChar     uint16
	ControlKeyState uint32
}

type windowsReadlineReader struct {
	stdin syscall.Handle
	ctrl  bool
	alt   bool
	proc  *syscall.LazyProc
}

func newWindowsReadlineReader() *windowsReadlineReader {
	kernel := syscall.NewLazyDLL("kernel32.dll")
	return &windowsReadlineReader{
		stdin: syscall.Stdin,
		proc:  kernel.NewProc("ReadConsoleInputW"),
	}
}

func (r *windowsReadlineReader) Read(buf []byte) (int, error) {
	for {
		var rec windowsInputRecord
		var read uint32
		ret, _, err := r.proc.Call(
			uintptr(r.stdin),
			uintptr(unsafe.Pointer(&rec)),
			1,
			uintptr(unsafe.Pointer(&read)),
		)
		if ret == 0 {
			return 0, err
		}
		if rec.EventType != windowsEventKey {
			continue
		}
		key := (*windowsKeyEventRecord)(unsafe.Pointer(&rec.Event[0]))
		if key.KeyDown == 0 {
			switch key.VirtualKeyCode {
			case windowsVKControl, windowsVKLControl, windowsVKRControl:
				r.ctrl = false
			case windowsVKMenu:
				r.alt = false
			}
			continue
		}
		if key.UnicodeChar == 0 {
			switch key.VirtualKeyCode {
			case windowsVKControl, windowsVKLControl, windowsVKRControl:
				r.ctrl = true
			case windowsVKMenu:
				r.alt = true
			case windowsVKLeft:
				return writeReadlineRune(buf, readline.CharBackward)
			case windowsVKRight:
				return writeReadlineRune(buf, readline.CharForward)
			case windowsVKUp:
				return writeReadlineRune(buf, readline.CharPrev)
			case windowsVKDown:
				return writeReadlineRune(buf, readline.CharNext)
			case windowsVKHome:
				return writeReadlineRune(buf, readline.CharLineStart)
			case windowsVKEnd:
				return writeReadlineRune(buf, readline.CharLineEnd)
			case windowsVKDelete:
				return writeReadlineRune(buf, readline.CharDelete)
			}
			continue
		}
		char := rune(key.UnicodeChar)
		if r.ctrl {
			switch char {
			case 'A':
				char = readline.CharLineStart
			case 'E':
				char = readline.CharLineEnd
			case 'R':
				char = readline.CharBckSearch
			case 'S':
				char = readline.CharFwdSearch
			}
		}
		if r.alt {
			buf[0] = readline.CharEsc
			n := copy(buf[1:], []byte(string(char)))
			return n + 1, nil
		}
		return writeReadlineRune(buf, char)
	}
}

func writeReadlineRune(buf []byte, char rune) (int, error) {
	return copy(buf, []byte(string(char))), nil
}

func (r *windowsReadlineReader) Close() error {
	return nil
}
