package main

import (
	"fmt"
	"os"
)

// cmdCompletion prints a static completion script for bash, zsh, or fish.
// Static rather than generated: the subcommand list is small and stable;
// a static script avoids pulling in a completion-gen library.
func cmdCompletion(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: shellboto completion <bash|zsh|fish>")
		return exitUsage
	}
	switch args[0] {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	case "fish":
		fmt.Print(fishCompletion)
	default:
		fmt.Fprintf(os.Stderr, "unknown shell %q (want bash|zsh|fish)\n", args[0])
		return exitUsage
	}
	return exitOK
}

const bashCompletion = `# shellboto bash completion
# Install: shellboto completion bash | sudo tee /etc/bash_completion.d/shellboto

_shellboto() {
    local cur prev verbs
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    verbs="doctor config audit db users service simulate mint-seed completion help -version -config"
    case "$prev" in
        shellboto)
            COMPREPLY=( $(compgen -W "$verbs" -- "$cur") );;
        config)
            COMPREPLY=( $(compgen -W "check" -- "$cur") );;
        audit)
            COMPREPLY=( $(compgen -W "verify search export replay" -- "$cur") );;
        db)
            COMPREPLY=( $(compgen -W "backup info vacuum" -- "$cur") );;
        users)
            COMPREPLY=( $(compgen -W "list tree" -- "$cur") );;
        service)
            COMPREPLY=( $(compgen -W "status start stop restart enable disable logs" -- "$cur") );;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") );;
    esac
}
complete -F _shellboto shellboto
`

const zshCompletion = `# shellboto zsh completion
# Install: shellboto completion zsh > "${fpath[1]}/_shellboto" && compinit

#compdef shellboto

_shellboto() {
    local -a verbs
    verbs=(
        'doctor:preflight check'
        'config:validate config'
        'audit:audit chain + forensics'
        'db:database ops'
        'users:list whitelisted users'
        'service:systemd convenience wrappers'
        'simulate:test danger matcher'
        'mint-seed:generate audit seed'
        'completion:print completion script'
        'help:show help'
    )
    if (( CURRENT == 2 )); then
        _describe 'command' verbs
        return
    fi
    case "$words[2]" in
        config)     _values 'config subcommand' 'check' ;;
        audit)      _values 'audit subcommand' 'verify' 'search' 'export' 'replay' ;;
        db)         _values 'db subcommand' 'backup' 'info' 'vacuum' ;;
        users)      _values 'users subcommand' 'list' 'tree' ;;
        service)    _values 'service verb' 'status' 'start' 'stop' 'restart' 'enable' 'disable' 'logs' ;;
        completion) _values 'shell' 'bash' 'zsh' 'fish' ;;
    esac
}
_shellboto "$@"
`

const fishCompletion = `# shellboto fish completion
# Install: shellboto completion fish > ~/.config/fish/completions/shellboto.fish

complete -c shellboto -f
complete -c shellboto -n '__fish_use_subcommand' -a doctor     -d 'preflight check'
complete -c shellboto -n '__fish_use_subcommand' -a config     -d 'validate config'
complete -c shellboto -n '__fish_use_subcommand' -a audit      -d 'audit chain + forensics'
complete -c shellboto -n '__fish_use_subcommand' -a db         -d 'database ops'
complete -c shellboto -n '__fish_use_subcommand' -a users      -d 'list whitelisted users'
complete -c shellboto -n '__fish_use_subcommand' -a service    -d 'systemd convenience wrappers'
complete -c shellboto -n '__fish_use_subcommand' -a simulate   -d 'test danger matcher'
complete -c shellboto -n '__fish_use_subcommand' -a mint-seed  -d 'generate audit seed'
complete -c shellboto -n '__fish_use_subcommand' -a completion -d 'print completion script'
complete -c shellboto -n '__fish_use_subcommand' -a help       -d 'show help'

complete -c shellboto -n '__fish_seen_subcommand_from config'     -a 'check'
complete -c shellboto -n '__fish_seen_subcommand_from audit'      -a 'verify search export replay'
complete -c shellboto -n '__fish_seen_subcommand_from db'         -a 'backup info vacuum'
complete -c shellboto -n '__fish_seen_subcommand_from users'      -a 'list tree'
complete -c shellboto -n '__fish_seen_subcommand_from service'    -a 'status start stop restart enable disable logs'
complete -c shellboto -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'
`
