package daemon

import (
	"fmt"
	"io"
	"net"
)

type socks5ReplyError struct {
	code byte
}

func (e socks5ReplyError) Error() string {
	return fmt.Sprintf("socks5 reply code %d", e.code)
}

func socks5FailureCode(err error) (byte, bool) {
	replyErr, ok := err.(socks5ReplyError)
	if !ok {
		return 0, false
	}
	return replyErr.code, true
}

func readSocks5Greeting(conn net.Conn) (bool, error) {
	buf := make([]byte, 512)
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return false, err
	}
	if buf[0] != 0x05 {
		return false, fmt.Errorf("invalid socks version")
	}

	nMethods := int(buf[1])
	if nMethods <= 0 || nMethods > 255 {
		return false, fmt.Errorf("invalid socks methods length")
	}
	if _, err := io.ReadFull(conn, buf[:nMethods]); err != nil {
		return false, err
	}

	for i := 0; i < nMethods; i++ {
		if buf[i] == 0x00 {
			return true, nil
		}
	}
	return false, nil
}

func readSocks5Request(conn net.Conn) (string, error) {
	buf := make([]byte, 512)
	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return "", err
	}
	if buf[0] != 0x05 || buf[1] != 0x01 {
		return "", socks5ReplyError{code: 0x07}
	}

	var host string
	switch buf[3] {
	case 0x01:
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			return "", err
		}
		host = net.IP(buf[:4]).String()
	case 0x03:
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return "", err
		}
		addrLen := int(buf[0])
		if _, err := io.ReadFull(conn, buf[:addrLen]); err != nil {
			return "", err
		}
		host = string(buf[:addrLen])
	case 0x04:
		if _, err := io.ReadFull(conn, buf[:16]); err != nil {
			return "", err
		}
		host = net.IP(buf[:16]).String()
	default:
		return "", socks5ReplyError{code: 0x08}
	}

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return "", err
	}
	port := int(buf[0])<<8 | int(buf[1])
	return net.JoinHostPort(host, fmt.Sprintf("%d", port)), nil
}

func writeSocks5NoAuth(conn net.Conn) error {
	_, err := conn.Write([]byte{0x05, 0x00})
	return err
}

func writeSocks5Failure(conn net.Conn, code byte) error {
	_, err := conn.Write([]byte{0x05, code, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	return err
}

func writeSocks5Success(conn net.Conn) error {
	_, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	return err
}
