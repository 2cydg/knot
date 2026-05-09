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
		"timeout /T 1 /NOBREAK",
		"Knot updated. Please run knot again.",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("WindowsHelperScript() missing %q in:\n%s", want, script)
		}
	}
}
