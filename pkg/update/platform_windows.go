//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type defaultInstaller struct{}

func (defaultInstaller) Install(extractedBinary, targetPath string) error {
	scriptPath := filepath.Join(os.TempDir(), fmt.Sprintf("knot-upgrade-%d.cmd", os.Getpid()))
	script := WindowsHelperScript(extractedBinary, targetPath)
	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		return err
	}
	cmd := exec.Command("cmd", "/C", "start", "", "/B", scriptPath)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(scriptPath)
		return err
	}
	return nil
}

func WindowsHelperScript(extractedBinary, targetPath string) string {
	tmpPath := targetPath + ".old"
	return fmt.Sprintf(`@echo off
setlocal
set "NEW=%s"
set "TARGET=%s"
set "OLD=%s"
for /L %%%%i in (1,1,50) do (
  move /Y "%%TARGET%%" "%%OLD%%" >NUL 2>NUL
  if not errorlevel 1 goto replace
  timeout /T 1 /NOBREAK >NUL
)
echo Failed to replace Knot: old executable is still locked.
exit /B 1
:replace
move /Y "%%NEW%%" "%%TARGET%%" >NUL
if errorlevel 1 (
  move /Y "%%OLD%%" "%%TARGET%%" >NUL 2>NUL
  echo Failed to install Knot update.
  exit /B 1
)
del /Q "%%OLD%%" >NUL 2>NUL
del /Q "%%~f0" >NUL 2>NUL
echo Knot updated. Please run knot again.
`, escapeCmdValue(extractedBinary), escapeCmdValue(targetPath), escapeCmdValue(tmpPath))
}

func escapeCmdValue(value string) string {
	return strings.ReplaceAll(value, `"`, `""`)
}
