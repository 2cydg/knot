package daemon

const osc7ShellHook = `__knot_osc7(){ __knot_uri=$PWD; __knot_uri=${__knot_uri//%/%25}; __knot_uri=${__knot_uri// /%20}; __knot_uri=${__knot_uri//#/%23}; __knot_uri=${__knot_uri//\?/%3F}; printf '\033]7;file://%s%s\007' "$(hostname 2>/dev/null || printf unknown)" "$__knot_uri"; }; if [ -n "${BASH_VERSION-}" ] && [ -z "${KNOT_OSC7_HOOK-}" ]; then export KNOT_OSC7_HOOK=1; if declare -p PROMPT_COMMAND 2>/dev/null | grep -q '^declare \-a'; then PROMPT_COMMAND=(__knot_osc7 "${PROMPT_COMMAND[@]}"); elif [ -n "${PROMPT_COMMAND-}" ]; then PROMPT_COMMAND="__knot_osc7; $PROMPT_COMMAND"; else PROMPT_COMMAND=__knot_osc7; fi; elif [ -n "${ZSH_VERSION-}" ] && [ -z "${KNOT_OSC7_HOOK-}" ]; then export KNOT_OSC7_HOOK=1; autoload -Uz add-zsh-hook 2>/dev/null; if command -v add-zsh-hook >/dev/null 2>&1; then add-zsh-hook precmd __knot_osc7; else case " ${precmd_functions[*]} " in *" __knot_osc7 "*) ;; *) precmd_functions=(${precmd_functions[@]} __knot_osc7) ;; esac; fi; fi; unset __knot_uri 2>/dev/null || true`

func injectOSC7Hook(session *Session) error {
	if session == nil {
		return nil
	}
	return session.WriteInput([]byte(osc7ShellHook + "; stty echo 2>/dev/null || true\n"))
}
