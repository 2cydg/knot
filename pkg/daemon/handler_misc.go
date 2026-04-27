package daemon

import (
	"encoding/json"
	"fmt"
	"knot/internal/protocol"
	"knot/pkg/config"
	"net"
	"os"
	"runtime"
	"time"
)

func (d *Daemon) handleStatusRequest(conn net.Conn) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	memUsage := getRSS()
	if memUsage == 0 {
		memUsage = m.Sys
	}

	poolStats := d.pool.GetStats()
	sessionsByPoolKey := d.sm.CountByPoolKey()
	for i := range poolStats {
		poolStats[i].Sessions = sessionsByPoolKey[poolStats[i].Key]
	}

	cfg, err := config.Load(d.crypto)
	if err != nil {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: load config failed"))
		return
	}

	forwardRules := forwardStatusesForServer(d.fm.ListRules(), cfg, "")
	activeForwardRules := 0
	for _, rule := range forwardRules {
		if rule.Status == "Active" {
			activeForwardRules++
		}
	}

	stats := protocol.StatusResponse{
		DaemonPID:          os.Getpid(),
		Uptime:             time.Since(d.startTime).Round(time.Second).String(),
		UDSPath:            d.socketPath,
		MemoryUsage:        memUsage,
		PoolStats:          poolStats,
		ActiveSessions:     d.sm.Count(),
		ActiveForwardRules: activeForwardRules,
		ForwardRules:       forwardRules,
		CryptoProvider:     d.crypto.Name(),
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
