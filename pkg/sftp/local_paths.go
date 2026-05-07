package sftp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func expandLocalHome(input string) (string, error) {
	if input == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(input, "~"+string(os.PathSeparator)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, input[2:]), nil
	}
	if os.PathSeparator == '\\' && strings.HasPrefix(input, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, filepath.FromSlash(input[2:])), nil
	}
	return input, nil
}

func resolveLocalPath(input, defaultDir string) (string, error) {
	hadDotSuffix := localPathHasDotSuffix(input)
	hadTrailingSeparator := localPathHasTrailingSeparator(input)

	input, err := expandLocalHome(input)
	if err != nil {
		return "", err
	}
	if defaultDir == "" || input == "" || isLocalPathAnchored(input) {
		return preserveLocalPathSuffix(input, hadDotSuffix, hadTrailingSeparator), nil
	}

	defaultDir, err = expandLocalHome(defaultDir)
	if err != nil {
		return "", err
	}
	if defaultDir == "" {
		return preserveLocalPathSuffix(input, hadDotSuffix, hadTrailingSeparator), nil
	}
	resolved := filepath.Join(defaultDir, filepath.FromSlash(input))
	return preserveLocalPathSuffix(resolved, hadDotSuffix, hadTrailingSeparator), nil
}

func ResolveConfiguredLocalPath(input string) (string, error) {
	return resolveLocalPath(input, "")
}

func isLocalPathAnchored(input string) bool {
	if input == "" {
		return false
	}
	if filepath.IsAbs(input) || strings.HasPrefix(input, "~") {
		return true
	}
	if len(input) >= 2 && isASCIIAlpha(input[0]) && input[1] == ':' {
		return true
	}
	return strings.HasPrefix(input, `\\`) || strings.HasPrefix(input, `//`)
}

func localPathHasTrailingSeparator(input string) bool {
	return hasTrailingLocalSeparator(input) && !isLocalRoot(input)
}

func trimTrailingLocalSeparators(input string) string {
	for hasTrailingLocalSeparator(input) && !isLocalRoot(input) {
		input = input[:len(input)-1]
	}
	return input
}

func hasTrailingLocalSeparator(input string) bool {
	return strings.HasSuffix(input, "/") || strings.HasSuffix(input, `\`)
}

func isLocalRoot(input string) bool {
	if input == "/" || input == `\` {
		return true
	}
	if len(input) == 3 && isASCIIAlpha(input[0]) && input[1] == ':' && (input[2] == '/' || input[2] == '\\') {
		return true
	}
	return false
}

func localPathHasDotSuffix(input string) bool {
	return strings.HasSuffix(input, "/.") || strings.HasSuffix(input, `\.`)
}

func trimLocalDotSuffix(input string) string {
	if localPathHasDotSuffix(input) {
		return input[:len(input)-2]
	}
	return input
}

func localBase(input string) string {
	input = trimTrailingLocalSeparators(input)
	input = filepath.Clean(input)
	base := filepath.Base(input)
	if base != "." && base != string(filepath.Separator) {
		return base
	}

	trimmed := strings.TrimRight(input, `/\`)
	idx := strings.LastIndexAny(trimmed, `/\`)
	if idx >= 0 {
		return trimmed[idx+1:]
	}
	return trimmed
}

func localCompletionDisplayPath(input string) string {
	if runtime.GOOS != "windows" {
		return input
	}
	return normalizeWindowsLocalDisplayPath(input)
}

func preserveLocalPathSuffix(input string, hadDotSuffix bool, hadTrailingSeparator bool) string {
	switch {
	case hadDotSuffix:
		base := trimTrailingLocalSeparators(input)
		if base == "" {
			base = "."
		}
		return base + string(os.PathSeparator) + "."
	case hadTrailingSeparator && !localPathHasTrailingSeparator(input):
		if isLocalRoot(input) {
			return input
		}
		return input + string(os.PathSeparator)
	default:
		return input
	}
}

func normalizeWindowsLocalDisplayPath(input string) string {
	input = strings.ReplaceAll(input, `\`, "/")
	if len(input) >= 2 && isASCIIAlpha(input[0]) && input[1] == ':' {
		rest := input[2:]
		if strings.HasPrefix(rest, "/") {
			rest = "/" + strings.TrimLeft(rest, "/")
		}
		return input[:2] + rest
	}
	return input
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
