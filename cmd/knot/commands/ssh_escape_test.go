package commands

import "testing"

func TestSSHEscapeParserRecognizesLineStartPause(t *testing.T) {
	parser := newSSHEscapeParser("~")
	results := parser.Process([]byte("~p"))

	if len(results) != 1 {
		t.Fatalf("results len = %d", len(results))
	}
	if results[0].Action != sshEscapeBroadcast || results[0].Request.Action != "pause" {
		t.Fatalf("result = %+v", results[0])
	}
}

func TestSSHEscapeParserIgnoresNonLineStart(t *testing.T) {
	parser := newSSHEscapeParser("~")
	results := parser.Process([]byte("abc~p"))

	if len(results) != 1 {
		t.Fatalf("results len = %d", len(results))
	}
	if results[0].Action != sshEscapeSend || string(results[0].Payload) != "abc~p" {
		t.Fatalf("result = %+v", results[0])
	}
}

func TestSSHEscapeParserSendsEscapedPrefix(t *testing.T) {
	parser := newSSHEscapeParser("~")
	results := parser.Process([]byte("~~"))

	if len(results) != 1 {
		t.Fatalf("results len = %d", len(results))
	}
	if results[0].Action != sshEscapeSend || string(results[0].Payload) != "~" {
		t.Fatalf("result = %+v", results[0])
	}
}

func TestSSHEscapeParserRecognizesAfterNewline(t *testing.T) {
	parser := newSSHEscapeParser("~")
	results := parser.Process([]byte("pwd\n~B"))

	if len(results) != 2 {
		t.Fatalf("results len = %d", len(results))
	}
	if results[0].Action != sshEscapeSend || string(results[0].Payload) != "pwd\n" {
		t.Fatalf("send result = %+v", results[0])
	}
	if results[1].Action != sshEscapeBroadcast || results[1].Request.Action != "leave" {
		t.Fatalf("escape result = %+v", results[1])
	}
}

func TestSSHEscapeParserStaysAtLineStartAfterLocalEscape(t *testing.T) {
	parser := newSSHEscapeParser("~")
	results := parser.Process([]byte("~?~p"))

	if len(results) != 2 {
		t.Fatalf("results len = %d", len(results))
	}
	if results[0].Action != sshEscapeHelp {
		t.Fatalf("first result = %+v", results[0])
	}
	if results[1].Action != sshEscapeBroadcast || results[1].Request.Action != "pause" {
		t.Fatalf("second result = %+v", results[1])
	}
}

func TestSSHEscapeParserDoesNotConsumeEnterAfterCommand(t *testing.T) {
	parser := newSSHEscapeParser("~")
	results := parser.Process([]byte("~p\rpwd\n"))

	if len(results) != 2 {
		t.Fatalf("results len = %d: %+v", len(results), results)
	}
	if results[0].Action != sshEscapeBroadcast || results[0].Request.Action != "pause" {
		t.Fatalf("pause result = %+v", results[0])
	}
	if results[1].Action != sshEscapeSend || string(results[1].Payload) != "\rpwd\n" {
		t.Fatalf("send result = %+v", results[1])
	}
}

func TestSSHEscapeParserTreatsCtrlCAsLineStart(t *testing.T) {
	parser := newSSHEscapeParser("~")
	results := parser.Process([]byte("abc\x03~p"))

	if len(results) != 2 {
		t.Fatalf("results len = %d: %+v", len(results), results)
	}
	if results[0].Action != sshEscapeSend || string(results[0].Payload) != "abc\x03" {
		t.Fatalf("send result = %+v", results[0])
	}
	if results[1].Action != sshEscapeBroadcast || results[1].Request.Action != "pause" {
		t.Fatalf("pause result = %+v", results[1])
	}
}

func TestSSHEscapeParserTreatsCtrlCAfterPendingEscapeAsLineStart(t *testing.T) {
	parser := newSSHEscapeParser("~")
	results := parser.Process([]byte("~\x03~p"))

	if len(results) != 2 {
		t.Fatalf("results len = %d: %+v", len(results), results)
	}
	if results[0].Action != sshEscapeSend || string(results[0].Payload) != "~\x03" {
		t.Fatalf("send result = %+v", results[0])
	}
	if results[1].Action != sshEscapeBroadcast || results[1].Request.Action != "pause" {
		t.Fatalf("pause result = %+v", results[1])
	}
}

func TestSSHEscapeParserDisabled(t *testing.T) {
	parser := newSSHEscapeParser("none")
	results := parser.Process([]byte("~p"))

	if len(results) != 1 {
		t.Fatalf("results len = %d", len(results))
	}
	if results[0].Action != sshEscapeSend || string(results[0].Payload) != "~p" {
		t.Fatalf("result = %+v", results[0])
	}
}

func TestSSHEscapeParserEmptyValueDefaultsToTilde(t *testing.T) {
	parser := newSSHEscapeParser("")
	results := parser.Process([]byte("~p"))

	if len(results) != 1 {
		t.Fatalf("results len = %d", len(results))
	}
	if results[0].Action != sshEscapeBroadcast || results[0].Request.Action != "pause" {
		t.Fatalf("result = %+v", results[0])
	}
}

func TestSSHEscapeParserCustomPrefix(t *testing.T) {
	parser := newSSHEscapeParser(",")
	results := parser.Process([]byte(",r"))

	if len(results) != 1 {
		t.Fatalf("results len = %d", len(results))
	}
	if results[0].Action != sshEscapeBroadcast || results[0].Request.Action != "resume" {
		t.Fatalf("result = %+v", results[0])
	}
}

func TestSSHEscapeParserRecognizesJoinWithGroup(t *testing.T) {
	parser := newSSHEscapeParser("~")
	results := parser.Process([]byte("~j cloud\rpwd\n"))

	if len(results) != 2 {
		t.Fatalf("results len = %d: %+v", len(results), results)
	}
	if results[0].Action != sshEscapeBroadcast || results[0].Request.Action != "join" || results[0].Request.Group != "cloud" {
		t.Fatalf("join result = %+v", results[0])
	}
	if results[1].Action != sshEscapeSend || string(results[1].Payload) != "\rpwd\n" {
		t.Fatalf("send result = %+v", results[1])
	}
}

func TestSSHEscapeParserJoinWithoutGroupPromptsAndConsumesNextLine(t *testing.T) {
	parser := newSSHEscapeParser("~")
	results := parser.Process([]byte("~j"))

	if len(results) != 1 {
		t.Fatalf("results len = %d", len(results))
	}
	if results[0].Action != sshEscapeLocalOutput || string(results[0].Payload) != "\r\n[knot] broadcast group: " {
		t.Fatalf("result = %+v", results[0])
	}

	results = parser.Process([]byte("cloud\rpwd\n"))
	if len(results) != 8 {
		t.Fatalf("results len = %d: %+v", len(results), results)
	}
	for i, want := range []string{"c", "l", "o", "u", "d"} {
		if results[i].Action != sshEscapeLocalOutput || string(results[i].Payload) != want {
			t.Fatalf("echo result %d = %+v", i, results[i])
		}
	}
	if results[5].Action != sshEscapeLocalOutput || string(results[5].Payload) != "\r\n" {
		t.Fatalf("newline result = %+v", results[5])
	}
	if results[6].Action != sshEscapeBroadcast || results[6].Request.Action != "join" || results[6].Request.Group != "cloud" {
		t.Fatalf("join result = %+v", results[6])
	}
	if results[7].Action != sshEscapeSend || string(results[7].Payload) != "pwd\n" {
		t.Fatalf("send result = %+v", results[7])
	}
}

func TestSSHEscapeParserJoinPromptEmptyGroupShowsHelp(t *testing.T) {
	parser := newSSHEscapeParser("~")
	results := parser.Process([]byte("~j\r\r"))

	if len(results) != 3 {
		t.Fatalf("results len = %d: %+v", len(results), results)
	}
	if results[0].Action != sshEscapeLocalOutput || string(results[0].Payload) != "\r\n[knot] broadcast group: " {
		t.Fatalf("prompt result = %+v", results[0])
	}
	if results[1].Action != sshEscapeLocalOutput || string(results[1].Payload) != "\r\n" {
		t.Fatalf("newline result = %+v", results[1])
	}
	if results[2].Action != sshEscapeHelp {
		t.Fatalf("help result = %+v", results[2])
	}
}

func TestSSHEscapeParserJoinPromptBackspace(t *testing.T) {
	parser := newSSHEscapeParser("~")
	_ = parser.Process([]byte("~j"))
	results := parser.Process([]byte("clx\x7foud\r"))

	if results[len(results)-1].Action != sshEscapeBroadcast || results[len(results)-1].Request.Group != "cloud" {
		t.Fatalf("results = %+v", results)
	}
	foundBackspace := false
	for _, result := range results {
		if result.Action == sshEscapeLocalOutput && string(result.Payload) == "\b \b" {
			foundBackspace = true
		}
	}
	if !foundBackspace {
		t.Fatalf("backspace echo missing: %+v", results)
	}
}

func TestSSHEscapeParserFlushPendingPrefix(t *testing.T) {
	parser := newSSHEscapeParser("~")
	results := parser.Process([]byte("~"))
	if len(results) != 0 {
		t.Fatalf("results = %+v", results)
	}

	results = parser.Flush()
	if len(results) != 1 {
		t.Fatalf("flush results len = %d", len(results))
	}
	if results[0].Action != sshEscapeSend || string(results[0].Payload) != "~" {
		t.Fatalf("result = %+v", results[0])
	}
}

func TestSSHEscapeHelpTextUsesCustomPrefix(t *testing.T) {
	got := sshEscapeHelpTextFor(",")
	want := "[broadcast escapes: ,j <group> join, ,B leave, ,p pause, ,r resume, ,? help, ,, send ,]"
	if got != want {
		t.Fatalf("sshEscapeHelpTextFor() = %q, want %q", got, want)
	}
}

func TestSSHEscapeParserHelpUsesCustomPrefix(t *testing.T) {
	parser := newSSHEscapeParser(",")
	results := parser.Process([]byte(",?"))

	if len(results) != 1 {
		t.Fatalf("results len = %d", len(results))
	}
	if results[0].Action != sshEscapeHelp || results[0].Message != sshEscapeHelpTextFor(",") {
		t.Fatalf("result = %+v", results[0])
	}
}
