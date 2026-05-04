package daemon

import (
	"io"
	"net"
	"time"
)

type readOnlyConn struct {
	io.Reader
}

func (c readOnlyConn) Read(p []byte) (int, error)       { return c.Reader.Read(p) }
func (c readOnlyConn) Write([]byte) (int, error)        { return 0, io.ErrClosedPipe }
func (c readOnlyConn) Close() error                     { return nil }
func (c readOnlyConn) LocalAddr() net.Addr              { return dummyAddr("local") }
func (c readOnlyConn) RemoteAddr() net.Addr             { return dummyAddr("remote") }
func (c readOnlyConn) SetDeadline(time.Time) error      { return nil }
func (c readOnlyConn) SetReadDeadline(time.Time) error  { return nil }
func (c readOnlyConn) SetWriteDeadline(time.Time) error { return nil }

type dummyAddr string

func (a dummyAddr) Network() string { return string(a) }
func (a dummyAddr) String() string  { return string(a) }
