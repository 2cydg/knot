package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ExitCodeError represents an error that carries a specific OS exit code.
type ExitCodeError struct {
	Code int
	Err  error
}

func (e *ExitCodeError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit code %d", e.Code)
}

// Formatter handles consistent output across commands, supporting both human-readable and JSON formats.
type Formatter struct {
	JSON bool
}

type JSONError struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// NewFormatter creates a new Formatter based on the global jsonOutput flag.
func NewFormatter() *Formatter {
	return &Formatter{JSON: jsonOutput}
}

// Render outputs the data either as JSON or via a provided human-readable render function.
func (f *Formatter) Render(data interface{}, humanRenderer func() error) error {
	if f.JSON {
		return f.RenderJSON(data, nil)
	}
	if humanRenderer != nil {
		return humanRenderer()
	}
	return nil
}

func (f *Formatter) RenderJSON(data interface{}, jsonErr *JSONError) error {
	if !f.JSON {
		return nil
	}
	payload := normalizeJSONPayload(data)
	if _, exists := payload["data"]; !exists {
		payload["data"] = payloadData(payload)
	}
	if jsonErr != nil {
		payload["ok"] = false
		payload["error"] = jsonErr
	} else {
		payload["ok"] = true
		payload["error"] = nil
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(payload)
}

// PrintError outputs an error message, potentially in JSON format if requested.
func (f *Formatter) PrintError(err error) {
	if f.JSON {
		res := map[string]interface{}{
			"ok":    false,
			"data":  nil,
			"error": NewJSONError(err),
		}
		encoder := json.NewEncoder(os.Stderr)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(res)
	} else {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}

func NewJSONError(err error) *JSONError {
	if err == nil {
		return nil
	}
	return &JSONError{
		Code:    ErrorCodeForError(err),
		Message: err.Error(),
	}
}

func ErrorCodeForError(err error) string {
	if err == nil {
		return ""
	}

	var exitErr *ExitCodeError
	if errors.As(err, &exitErr) {
		if exitErr.Code != 0 {
			return "remote_exit_nonzero"
		}
	}

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "server alias") && strings.Contains(msg, "not found"):
		return "alias_not_found"
	case strings.Contains(msg, "authentication failed") ||
		strings.Contains(msg, "unable to authenticate"):
		return "auth_failed"
	case strings.Contains(msg, "permission denied"):
		return "permission_denied"
	case strings.Contains(msg, "remote host identification has changed"):
		return "host_key_changed"
	case strings.Contains(msg, "invalid host key policy"):
		return "invalid_argument"
	case strings.Contains(msg, "host key"):
		return "host_key_required"
	case strings.Contains(msg, "timed out") || strings.Contains(msg, "timeout"):
		return "timeout"
	case strings.Contains(msg, "daemon is not running") ||
		strings.Contains(msg, "knot.sock") ||
		strings.Contains(msg, "connection refused"):
		return "daemon_not_running"
	case strings.Contains(msg, "already exists") || strings.Contains(msg, "file exists"):
		return "path_exists"
	case strings.Contains(msg, "no such file") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "does not exist"):
		return "path_not_found"
	case strings.Contains(msg, "remote-to-remote") ||
		strings.Contains(msg, "local-to-local") ||
		strings.Contains(msg, "same alias") ||
		strings.Contains(msg, "not supported") ||
		strings.Contains(msg, "unsupported"):
		return "unsupported_operation"
	case strings.Contains(msg, "invalid") ||
		strings.Contains(msg, "required") ||
		strings.Contains(msg, "unknown setting"):
		return "invalid_argument"
	case strings.Contains(msg, "connection lost") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		msg == "eof":
		return "connection_lost"
	default:
		return "unknown"
	}
}

func normalizeJSONPayload(data interface{}) map[string]interface{} {
	if data == nil {
		return map[string]interface{}{"data": nil}
	}

	if m, ok := data.(map[string]interface{}); ok {
		res := make(map[string]interface{}, len(m)+2)
		for k, v := range m {
			res[k] = v
		}
		return res
	}

	raw, err := json.Marshal(data)
	if err == nil {
		var obj map[string]interface{}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&obj); err == nil && obj != nil {
			return obj
		}
	}

	return map[string]interface{}{"data": data}
}

func payloadData(payload map[string]interface{}) map[string]interface{} {
	data := make(map[string]interface{}, len(payload))
	for k, v := range payload {
		data[k] = v
	}
	return data
}
