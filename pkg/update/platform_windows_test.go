//go:build windows

package update

import (
	"strings"
	"testing"
)

func TestWindowsHelperScript(t *testing.T) {
	script := WindowsHelperScript(`C:\Temp\knot.exe`, `C:\Tools\knot.exe`)
	for _, want := range []string{
		`set "NEW=C:\Temp\knot.exe"`,
		`set "TARGET=C:\Tools\knot.exe"`,
		`set "LOG=`,
		"timeout /T 1 /NOBREAK",
		"Failed to replace Knot: old executable is still locked.",
		`del /Q "%NEW%"`,
		"Knot updated. Please run knot again.",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("WindowsHelperScript() missing %q in:\n%s", want, script)
		}
	}
	for _, forbidden := range []string{
		`del /Q "%~f0"`,
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("WindowsHelperScript() contains forbidden %q in:\n%s", forbidden, script)
		}
	}
}
