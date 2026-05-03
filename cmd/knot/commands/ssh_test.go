package commands

import (
	"knot/internal/protocol"
	"testing"
)

func TestFormatBroadcastNotify(t *testing.T) {
	got := formatBroadcastNotify([]byte(`{"message":"[broadcast: paused]"}`))
	if got != "[broadcast: paused]" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatBroadcastNotifyFallsBackToPayload(t *testing.T) {
	got := formatBroadcastNotify([]byte("plain notify"))
	if got != "plain notify" {
		t.Fatalf("got %q", got)
	}
}

func TestSSHRequestBroadcastFields(t *testing.T) {
	req := protocol.SSHRequest{
		Alias:          "web",
		BroadcastGroup: "deploy",
		Escape:         "none",
		IsInteractive:  true,
	}
	if req.BroadcastGroup != "deploy" || req.Escape != "none" {
		t.Fatalf("req = %+v", req)
	}
}

func TestSSHTerminalTitle(t *testing.T) {
	tests := []struct {
		alias string
		group string
		want  string
	}{
		{alias: "web-prod", want: "web-prod"},
		{alias: "web-prod", group: "cloud", want: "web-prod [📢 cloud]"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := sshTerminalTitle(tt.alias, tt.group); got != tt.want {
				t.Fatalf("sshTerminalTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsTerminalResponse(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		want    bool
	}{
		{name: "osc color response", payload: []byte("\x1b]11;rgb:0c0c/0c0c/0c0c\x07"), want: true},
		{name: "cursor position response", payload: []byte("\x1b[1;1R"), want: true},
		{name: "primary device attributes response", payload: []byte("\x1b[?64;1;2;6;9;15;18;21;22c"), want: true},
		{name: "secondary device attributes response", payload: []byte("\x1b[>41;3800;0c"), want: true},
		{name: "terminal size response", payload: []byte("\x1b[8;24;80t"), want: true},
		{name: "kitty keyboard response", payload: []byte("\x1b[?1u"), want: true},
		{name: "decrqss cursor style response", payload: []byte("\x1b[1$r"), want: false},
		{name: "decrqss mode response", payload: []byte("\x1b[?2004;1$y"), want: true},
		{name: "arrow key input", payload: []byte("\x1b[A"), want: false},
		{name: "bracketed paste start", payload: []byte("\x1b[200~"), want: false},
		{name: "keyboard escape alone", payload: []byte("\x1b"), want: false},
		{name: "enter key", payload: []byte("\r"), want: false},
		{name: "plain text", payload: []byte("duf\n"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTerminalResponse(tt.payload); got != tt.want {
				t.Fatalf("isTerminalResponse(%q) = %v, want %v", tt.payload, got, tt.want)
			}
		})
	}
}

func TestTerminalResponseClassifierSplitOSC(t *testing.T) {
	classifier := terminalResponseClassifier{}
	chunks := [][]byte{
		[]byte("\x1b]11;rgb:"),
		[]byte("0c0c/0c0c/0c0c"),
		[]byte("\x07"),
	}
	for i, chunk := range chunks {
		if !classifier.IsTerminalResponse(chunk) {
			t.Fatalf("chunk %d was not classified as terminal response: %q", i, chunk)
		}
	}
	if classifier.IsTerminalResponse([]byte("pwd\n")) {
		t.Fatal("plain input after OSC should not be terminal response")
	}
}

func TestTerminalResponseClassifierSplitCSI(t *testing.T) {
	classifier := terminalResponseClassifier{}
	if !classifier.IsTerminalResponse([]byte("\x1b[?64;1;2")) {
		t.Fatal("initial CSI chunk should be terminal response")
	}
	if !classifier.IsTerminalResponse([]byte(";6;9;15;18;21;22c")) {
		t.Fatal("final CSI chunk should be terminal response")
	}
	if classifier.IsTerminalResponse([]byte("\x1b[A")) {
		t.Fatal("arrow key input should not be terminal response")
	}
}

func TestTerminalResponseClassifierSplitCSIWithIntermediate(t *testing.T) {
	classifier := terminalResponseClassifier{}
	if !classifier.IsTerminalResponse([]byte("\x1b[?2004;")) {
		t.Fatal("initial CSI chunk should be terminal response")
	}
	if !classifier.IsTerminalResponse([]byte("1$y")) {
		t.Fatal("final CSI chunk with intermediate should be terminal response")
	}
	if classifier.IsTerminalResponse([]byte("\x1b[A")) {
		t.Fatal("arrow key input should not be terminal response after split CSI")
	}
}
