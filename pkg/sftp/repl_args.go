package sftp

import "fmt"

type transferArgs struct {
	recursive bool
	paths     []string
}

func parseTransferArgs(args []string) (transferArgs, error) {
	var parsed transferArgs
	for i, arg := range args {
		switch arg {
		case "-r", "--recursive":
			parsed.recursive = true
		case "--":
			parsed.paths = append(parsed.paths, args[i+1:]...)
			return parsed, nil
		default:
			if len(arg) > 0 && arg[0] == '-' {
				return parsed, fmt.Errorf("unknown option %s", arg)
			}
			parsed.paths = append(parsed.paths, arg)
		}
	}
	return parsed, nil
}
