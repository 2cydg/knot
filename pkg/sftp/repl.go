package sftp

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/peterh/liner"
	"github.com/pkg/sftp"
)

// RunREPL starts an interactive SFTP shell.
func RunREPL(client *sftp.Client, alias string) error {
	line := liner.NewLiner()
	defer line.Close()
	line.SetCtrlCAborts(true)

	// Set up history
	historyPath := filepath.Join(os.TempDir(), ".knot_sftp_history")
	if f, err := os.Open(historyPath); err == nil {
		line.ReadHistory(f)
		f.Close()
	}
	defer func() {
		if f, err := os.Create(historyPath); err == nil {
			line.WriteHistory(f)
			f.Close()
		}
	}()

	cwd, err := client.Getwd()
	if err != nil {
		cwd = "/"
	}

	fmt.Printf("Connected to %s via SFTP. Type 'help' for commands.\n", alias)

	for {
		prompt := fmt.Sprintf("sftp:%s> ", cwd)
		input, err := line.Prompt(prompt)
		if err != nil {
			if err == liner.ErrPromptAborted || err == io.EOF {
				fmt.Println()
				return nil
			}
			return err
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		line.AppendHistory(input)
		args := strings.Fields(input)
		cmd := args[0]

		switch cmd {
		case "help", "?":
			printHelp()
		case "exit", "quit", "bye":
			return nil
		case "ls":
			p := cwd
			if len(args) > 1 {
				p = resolvePath(cwd, args[1])
			}
			files, err := client.ReadDir(p)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}
			for _, f := range files {
				name := f.Name()
				if f.IsDir() {
					name += "/"
				}
				fmt.Printf("%-20s %10d %s\n", name, f.Size(), f.ModTime().Format("Jan _2 15:04"))
			}
		case "pwd":
			fmt.Println(cwd)
		case "cd":
			if len(args) < 2 {
				fmt.Println("Usage: cd <path>")
				continue
			}
			newPath := resolvePath(cwd, args[1])
			stat, err := client.Stat(newPath)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}
			if !stat.IsDir() {
				fmt.Printf("Error: %s is not a directory\n", newPath)
				continue
			}
			cwd = newPath
		case "get":
			if len(args) < 2 {
				fmt.Println("Usage: get <remote_path> [local_path]")
				continue
			}
			remotePath := resolvePath(cwd, args[1])
			localPath := path.Base(remotePath)
			if len(args) > 2 {
				localPath = args[2]
			}
			if err := Download(client, remotePath, localPath); err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Println("Download complete.")
			}
		case "put":
			if len(args) < 2 {
				fmt.Println("Usage: put <local_path> [remote_path]")
				continue
			}
			localPath := args[1]
			remotePath := path.Join(cwd, filepath.Base(localPath))
			if len(args) > 2 {
				remotePath = resolvePath(cwd, args[2])
			}
			if err := Upload(client, localPath, remotePath); err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Println("Upload complete.")
			}
		case "rm":
			if len(args) < 2 {
				fmt.Println("Usage: rm <path>")
				continue
			}
			p := resolvePath(cwd, args[1])
			if err := client.Remove(p); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "mkdir":
			if len(args) < 2 {
				fmt.Println("Usage: mkdir <path>")
				continue
			}
			p := resolvePath(cwd, args[1])
			if err := client.MkdirAll(p); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "rmdir":
			if len(args) < 2 {
				fmt.Println("Usage: rmdir <path>")
				continue
			}
			p := resolvePath(cwd, args[1])
			if err := client.RemoveDirectory(p); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		default:
			fmt.Printf("Unknown command: %s. Type 'help' for assistance.\n", cmd)
		}
	}
}

func resolvePath(cwd, p string) string {
	if path.IsAbs(p) {
		return path.Clean(p)
	}
	return path.Clean(path.Join(cwd, p))
}

func printHelp() {
	fmt.Println("Available commands:")
	fmt.Println("  ls [path]          List directory contents")
	fmt.Println("  cd <path>          Change remote directory")
	fmt.Println("  pwd                Print remote working directory")
	fmt.Println("  get <remote> [loc] Download file")
	fmt.Println("  put <local> [rem]  Upload file")
	fmt.Println("  rm <path>          Remove remote file")
	fmt.Println("  mkdir <path>       Create remote directory")
	fmt.Println("  rmdir <path>       Remove remote directory")
	fmt.Println("  exit/quit/bye      Exit SFTP shell")
	fmt.Println("  help/?             Show this help")
}
