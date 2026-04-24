package commands

import "github.com/chzyer/readline"

func readLineWithPrompt(rl *readline.Instance, prompt string) (string, error) {
	rl.SetPrompt(prompt)
	rl.Refresh()
	return rl.Readline()
}
