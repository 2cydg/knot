package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	logsTail   int
	logsFollow bool
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View daemon logs",
	Long:  "View knot daemon logs. Supports --tail to show the last N lines and -f to follow log output.",
	Example: `  knot logs                  # Show last 100 lines
  knot logs --tail=50        # Show last 50 lines
  knot logs -f               # Follow log output
  knot logs --tail=20 -f     # Show last 20 lines and follow`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		logPath := filepath.Join(home, ".config/knot/knot.log")

		if logsFollow {
			return followLogs(logPath)
		}

		return tailLogs(logPath)
	},
}

func tailLogs(logPath string) error {
	lines, err := readLastNLines(logPath, logsTail)
	if err != nil {
		return err
	}

	for _, line := range lines {
		fmt.Print(line)
	}
	return nil
}

func followLogs(logPath string) error {
	// First, print the last N lines
	lines, err := readLastNLines(logPath, logsTail)
	if err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Print(line)
	}

	// Then follow the file for new content
	return watchFile(logPath)
}

func readLastNLines(path string, n int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w (path: %s)", err, path)
	}
	defer file.Close()

	var allLines []string
	scanner := bufio.NewScanner(file)
	// Increase buffer size for very long lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text()+"\n")
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading log file: %w", err)
	}

	if len(allLines) <= n {
		return allLines, nil
	}
	return allLines[len(allLines)-n:], nil
}

func watchFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	// Seek to end
	if _, err := file.Seek(0, 2); err != nil {
		return fmt.Errorf("failed to seek in log file: %w", err)
	}

	reader := bufio.NewReader(file)

	// Check if file has been truncated (rotated)
	var lastSize int64
	if fi, err := file.Stat(); err == nil {
		lastSize = fi.Size()
	}

	for {
		line, err := reader.ReadString('\n')
		if err == nil && line != "" {
			fmt.Print(line)
			continue
		}

		// Check for file rotation/truncation
		if fi, err := file.Stat(); err == nil {
			if fi.Size() < lastSize {
				// File was truncated/rotated, reopen
				file.Close()
				newFile, err := os.Open(path)
				if err != nil {
					return fmt.Errorf("failed to reopen log file: %w", err)
				}
				file = newFile
				reader = bufio.NewReader(file)
				lastSize = 0
				continue
			}
			lastSize = fi.Size()
		}

		time.Sleep(250 * time.Millisecond)
	}
}

func extractTime(line string) (time.Time, error) {
	// slog text format: time=2024-01-15T10:30:00.000Z level=INFO msg=...
	if idx := strings.Index(line, "time="); idx != -1 {
		rest := line[idx+5:]
		if end := strings.IndexByte(rest, ' '); end != -1 {
			ts := rest[:end]
			for _, layout := range []string{
				time.RFC3339Nano,
				time.RFC3339,
				"2006-01-02T15:04:05.000Z07:00",
				"2006-01-02T15:04:05Z07:00",
				"2006-01-02T15:04:05",
			} {
				if t, err := time.Parse(layout, ts); err == nil {
					return t, nil
				}
			}
		}
	}
	return time.Time{}, fmt.Errorf("no timestamp found")
}

func init() {
	logsCmd.Flags().IntVarP(&logsTail, "tail", "t", 100, "Number of lines to show from the end of the logs")
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	logsCmd.GroupID = advancedGroup.ID
	rootCmd.AddCommand(logsCmd)
}

// Convert int to string helper
func intToString(n int) string {
	return strconv.Itoa(n)
}