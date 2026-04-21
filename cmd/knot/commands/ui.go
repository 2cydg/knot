package commands

import (
	"encoding/json"
	"fmt"
	"os"
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

// NewFormatter creates a new Formatter based on the global jsonOutput flag.
func NewFormatter() *Formatter {
	return &Formatter{JSON: jsonOutput}
}

// Render outputs the data either as JSON or via a provided human-readable render function.
func (f *Formatter) Render(data interface{}, humanRenderer func() error) error {
	if f.JSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(data)
	}
	if humanRenderer != nil {
		return humanRenderer()
	}
	return nil
}

// PrintError outputs an error message, potentially in JSON format if requested.
func (f *Formatter) PrintError(err error) {
	if f.JSON {
		res := map[string]interface{}{
			"error": err.Error(),
		}
		encoder := json.NewEncoder(os.Stderr)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(res)
	} else {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}
