package protocol

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBroadcastProtocolTypesDoNotConflict(t *testing.T) {
	seen := map[uint8]string{
		TypeReq:              "TypeReq",
		TypeResp:             "TypeResp",
		TypeData:             "TypeData",
		TypeSignal:           "TypeSignal",
		TypeHostKeyConfirm:   "TypeHostKeyConfirm",
		TypeSFTPReq:          "TypeSFTPReq",
		TypeDisconnect:       "TypeDisconnect",
		TypeStatusReq:        "TypeStatusReq",
		TypeStatusResp:       "TypeStatusResp",
		TypeForwardReq:       "TypeForwardReq",
		TypeForwardListReq:   "TypeForwardListReq",
		TypeForwardListResp:  "TypeForwardListResp",
		TypeForwardNotify:    "TypeForwardNotify",
		TypeClearReq:         "TypeClearReq",
		TypeClearResp:        "TypeClearResp",
		TypeExecReq:          "TypeExecReq",
		TypeExecResp:         "TypeExecResp",
		TypeAuthChallenge:    "TypeAuthChallenge",
		TypeAuthResponse:     "TypeAuthResponse",
		TypeAuthRetryAbort:   "TypeAuthRetryAbort",
		TypeSessionListReq:   "TypeSessionListReq",
		TypeSessionListResp:  "TypeSessionListResp",
		TypeSessionCWDNotify: "TypeSessionCWDNotify",
	}
	for typ, name := range map[uint8]string{
		TypeBroadcastReq:    "TypeBroadcastReq",
		TypeBroadcastResp:   "TypeBroadcastResp",
		TypeBroadcastNotify: "TypeBroadcastNotify",
	} {
		if existing, ok := seen[typ]; ok {
			t.Fatalf("%s conflicts with %s at 0x%x", name, existing, typ)
		}
		seen[typ] = name
	}
}

func TestBroadcastJSONFieldNames(t *testing.T) {
	joinedAt := time.Date(2026, 5, 2, 1, 2, 3, 0, time.UTC)
	resp := BroadcastResponse{
		Groups: []BroadcastGroupInfo{{
			Group:     "deploy",
			Members:   2,
			Active:    1,
			Paused:    1,
			CreatedAt: joinedAt,
		}},
		Members: []BroadcastMemberInfo{{
			SessionID: "abc123",
			Alias:     "web",
			Remote:    "user@example:22",
			State:     "active",
			JoinedAt:  joinedAt,
		}},
		Message: "ok",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}

	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"groups", "members", "message"} {
		if _, ok := fields[field]; !ok {
			t.Fatalf("missing field %q in %s", field, data)
		}
	}

	var roundTrip BroadcastResponse
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if roundTrip.Groups[0].CreatedAt != joinedAt {
		t.Fatalf("created_at = %s, want %s", roundTrip.Groups[0].CreatedAt, joinedAt)
	}
	if roundTrip.Members[0].SessionID != "abc123" {
		t.Fatalf("session_id = %q", roundTrip.Members[0].SessionID)
	}
}

func TestBroadcastNotifyJSONFieldNames(t *testing.T) {
	notify := BroadcastNotify{
		Group:     "deploy",
		SessionID: "abc123",
		Action:    "pause",
		State:     "paused",
		Message:   "[broadcast: paused]",
		Level:     "info",
	}

	data, err := json.Marshal(notify)
	if err != nil {
		t.Fatal(err)
	}

	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"group", "session_id", "action", "state", "message", "level"} {
		if _, ok := fields[field]; !ok {
			t.Fatalf("missing field %q in %s", field, data)
		}
	}
}

func TestSSHRequestBroadcastJSONFields(t *testing.T) {
	req := SSHRequest{
		Alias:          "web",
		Term:           "xterm",
		Rows:           24,
		Cols:           80,
		IsInteractive:  true,
		BroadcastGroup: "deploy",
		Escape:         "~",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}

	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["broadcast_group"] != "deploy" {
		t.Fatalf("broadcast_group = %v", fields["broadcast_group"])
	}
	if fields["escape"] != "~" {
		t.Fatalf("escape = %v", fields["escape"])
	}
}
