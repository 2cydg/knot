package daemon

import (
	"encoding/json"
	"knot/internal/protocol"
	"net"
	"strings"
	"testing"
	"time"
)

func TestHandleBroadcastListResponse(t *testing.T) {
	d := testDaemonWithBroadcast()
	session := d.sm.Add("server", "web", nil, nil)
	if err := d.bm.Join("deploy", session); err != nil {
		t.Fatal(err)
	}

	resp := sendBroadcastRequest(t, d, protocol.BroadcastRequest{Action: "list"})

	if resp.Error != "" {
		t.Fatalf("error = %s", resp.Error)
	}
	if len(resp.Groups) != 1 || resp.Groups[0].Group != "deploy" {
		t.Fatalf("groups = %+v", resp.Groups)
	}
}

func TestHandleBroadcastJoinNotifiesTarget(t *testing.T) {
	d := testDaemonWithBroadcast()
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	session := d.sm.Add("server", "web", serverConn, nil)

	respCh := make(chan protocol.BroadcastResponse, 1)
	go func() {
		respCh <- sendBroadcastRequest(t, d, protocol.BroadcastRequest{
			Action:   "join",
			Group:    "deploy",
			Selector: session.ID,
		})
	}()

	msg, err := protocol.ReadMessage(clientConn)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Header.Type != protocol.TypeBroadcastNotify {
		t.Fatalf("type = %d", msg.Header.Type)
	}
	var notify protocol.BroadcastNotify
	if err := json.Unmarshal(msg.Payload, &notify); err != nil {
		t.Fatal(err)
	}
	if notify.Group != "deploy" || !strings.Contains(notify.Message, "joined deploy") {
		t.Fatalf("notify = %+v", notify)
	}

	resp := <-respCh
	if resp.Error != "" {
		t.Fatalf("error = %s", resp.Error)
	}
	if !strings.Contains(resp.Message, "joined broadcast group deploy") {
		t.Fatalf("message = %q", resp.Message)
	}
}

func TestHandleBroadcastDisbandNotifiesAllMembers(t *testing.T) {
	d := testDaemonWithBroadcast()
	serverA, clientA := net.Pipe()
	defer serverA.Close()
	defer clientA.Close()
	serverB, clientB := net.Pipe()
	defer serverB.Close()
	defer clientB.Close()
	a := d.sm.Add("server", "web-a", serverA, nil)
	b := d.sm.Add("server", "web-b", serverB, nil)
	if err := d.bm.Join("deploy", a); err != nil {
		t.Fatal(err)
	}
	if err := d.bm.Join("deploy", b); err != nil {
		t.Fatal(err)
	}

	respCh := make(chan protocol.BroadcastResponse, 1)
	go func() {
		respCh <- sendBroadcastRequest(t, d, protocol.BroadcastRequest{
			Action: "disband",
			Group:  "deploy",
		})
	}()

	for _, conn := range []net.Conn{clientA, clientB} {
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			t.Fatal(err)
		}
		if msg.Header.Type != protocol.TypeBroadcastNotify {
			t.Fatalf("type = %d", msg.Header.Type)
		}
	}

	resp := <-respCh
	if resp.Error != "" {
		t.Fatalf("error = %s", resp.Error)
	}
	if groups := d.bm.List(); len(groups) != 0 {
		t.Fatalf("groups = %+v", groups)
	}
}

func TestHandleBroadcastAmbiguousSelectorDoesNotMutate(t *testing.T) {
	d := testDaemonWithBroadcast()
	d.sm.Add("server-a", "web", nil, nil)
	d.sm.Add("server-b", "web", nil, nil)

	resp := sendBroadcastRequest(t, d, protocol.BroadcastRequest{
		Action:   "join",
		Group:    "deploy",
		Selector: "web",
	})

	if resp.Error == "" {
		t.Fatal("expected error")
	}
	if len(resp.Members) != 2 {
		t.Fatalf("members = %+v", resp.Members)
	}
	if groups := d.bm.List(); len(groups) != 0 {
		t.Fatalf("groups = %+v", groups)
	}
}

func sendBroadcastRequest(t *testing.T, d *Daemon, req protocol.BroadcastRequest) protocol.BroadcastResponse {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	payload, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		d.handleBroadcastRequest(serverConn, payload)
		close(done)
	}()

	if err := clientConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	msg, err := protocol.ReadMessage(clientConn)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Header.Type != protocol.TypeBroadcastResp {
		t.Fatalf("type = %d", msg.Header.Type)
	}
	var resp protocol.BroadcastResponse
	if err := json.Unmarshal(msg.Payload, &resp); err != nil {
		t.Fatal(err)
	}
	<-done
	return resp
}

func testDaemonWithBroadcast() *Daemon {
	return &Daemon{
		sm: NewSessionManager(),
		bm: NewBroadcastManager(),
	}
}
