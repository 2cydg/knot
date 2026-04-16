package daemon

import (
	"encoding/json"
	"fmt"
	"knot/internal/protocol"
	"net"
	"os"
	"runtime"
	"time"
)

func (d *Daemon) handleSessionListRequest(conn net.Conn, alias string) {
	sessions := d.sm.ListByAlias(alias)
	data, err := json.Marshal(sessions)
	if err != nil {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: marshal sessions failed"))
		return
	}
	protocol.WriteMessage(conn, protocol.TypeResp, 0, data)
}

func (d *Daemon) handleStatusRequest(conn net.Conn) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	stats := protocol.StatusResponse{
		DaemonPID:      os.Getpid(),
		Uptime:         time.Since(d.startTime).Round(time.Second).String(),
		UDSPath:        d.socketPath,
		MemoryUsage:    m.Alloc,
		PoolStats:      d.pool.GetStats(),
		ActiveSessions: d.sm.Count(),
		CryptoProvider: d.crypto.Name(),
	}

	data, err := json.Marshal(stats)
	if err != nil {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: marshal status failed"))
		return
	}
	protocol.WriteMessage(conn, protocol.TypeStatusResp, 0, data)
}

func (d *Daemon) handleClearRequest(conn net.Conn) {
	// 1. Clear sessions first (consistency)
	d.sm.Clear()
	// 2. Close all physical connections
	count := d.pool.CloseAll()
	// 3. Respond with count in Reserved field and TypeClearResp
	protocol.WriteMessage(conn, protocol.TypeClearResp, uint8(count), []byte(fmt.Sprintf("ok: %d connections cleared", count)))
}
