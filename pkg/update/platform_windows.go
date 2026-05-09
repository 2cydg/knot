//go:build windows

package update

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type defaultInstaller struct{}

func (defaultInstaller) Install(extractedBinary, targetPath string) error {
	scriptPath := filepath.Join(os.TempDir(), fmt.Sprintf("knot-upgrade-%d.cmd", os.Getpid()))
	stagedBinary := filepath.Join(os.TempDir(), fmt.Sprintf("knot-upgrade-%d.exe", os.Getpid()))
	if err := copyFile(extractedBinary, stagedBinary, 0755); err != nil {
		return err
	}
	script := WindowsHelperScript(stagedBinary, targetPath)
	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		_ = os.Remove(stagedBinary)
		return err
	}
	cmd := exec.Command("cmd", "/C", "start", "Knot Upgrade", "cmd", "/C", "call", scriptPath)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(scriptPath)
		_ = os.Remove(stagedBinary)
		return err
	}
	return nil
}

func copyFile(srcPath, dstPath string, perm os.FileMode) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func WindowsHelperScript(extractedBinary, targetPath string) string {
	tmpPath := targetPath + ".old"
	logPath := filepath.Join(os.TempDir(), "knot-upgrade.log")
	return fmt.Sprintf(`@echo off
setlocal
set "NEW=%s"
set "TARGET=%s"
set "OLD=%s"
set "LOG=%s"
echo Starting Knot update at %%DATE%% %%TIME%%>"%%LOG%%"
for /L %%%%i in (1,1,50) do (
  move /Y "%%TARGET%%" "%%OLD%%" >NUL 2>NUL
  if not errorlevel 1 goto replace
  timeout /T 1 /NOBREAK >NUL
)
echo Failed to replace Knot: old executable is still locked.
echo Failed to replace Knot: old executable is still locked.>>"%%LOG%%"
exit /B 1
:replace
move /Y "%%NEW%%" "%%TARGET%%" >NUL
if errorlevel 1 (
  move /Y "%%OLD%%" "%%TARGET%%" >NUL 2>NUL
  echo Failed to install Knot update.
  echo Failed to install Knot update.>>"%%LOG%%"
  exit /B 1
)
del /Q "%%OLD%%" >NUL 2>NUL
del /Q "%%NEW%%" >NUL 2>NUL
echo Knot updated. Please run knot again.
echo Knot updated. Please run knot again.>>"%%LOG%%"
`, escapeCmdValue(extractedBinary), escapeCmdValue(targetPath), escapeCmdValue(tmpPath), escapeCmdValue(logPath))
}

func escapeCmdValue(value string) string {
	return strings.ReplaceAll(value, `"`, `""`)
}
