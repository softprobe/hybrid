package main

import (
	"fmt"
	"io"
)

// runCompletion emits a shell-completion script for bash, zsh, or fish. The
// scripts are intentionally small: they enumerate the documented top-level
// commands and common flags (--runtime-url, --session, --json, --verbose,
// --quiet, --help, --version) without shelling out to `softprobe` at
// completion time. Users can regenerate on every release.
func runCompletion(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		_, _ = fmt.Fprintln(stderr, "usage: softprobe completion {bash|zsh|fish}")
		return exitInvalidArgs
	}
	switch args[0] {
	case "bash":
		_, _ = io.WriteString(stdout, bashCompletion)
	case "zsh":
		_, _ = io.WriteString(stdout, zshCompletion)
	case "fish":
		_, _ = io.WriteString(stdout, fishCompletion)
	default:
		_, _ = fmt.Fprintf(stderr, "completion: unsupported shell %q\n", args[0])
		return exitInvalidArgs
	}
	return exitOK
}

const bashCompletion = `# bash completion for softprobe
_softprobe_cmds="doctor inspect generate session validate replay suite capture scrub export completion --version version"
_softprobe_session_cmds="start stats close load-case rules policy"
_softprobe_inspect_cmds="case session"
_softprobe_validate_cmds="case rules suite"
_softprobe_suite_cmds="run validate diff"

_softprobe() {
  local cur prev
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  case "$prev" in
    softprobe) COMPREPLY=($(compgen -W "$_softprobe_cmds" -- "$cur")); return ;;
    session)   COMPREPLY=($(compgen -W "$_softprobe_session_cmds" -- "$cur")); return ;;
    inspect)   COMPREPLY=($(compgen -W "$_softprobe_inspect_cmds" -- "$cur")); return ;;
    validate)  COMPREPLY=($(compgen -W "$_softprobe_validate_cmds" -- "$cur")); return ;;
    suite)     COMPREPLY=($(compgen -W "$_softprobe_suite_cmds" -- "$cur")); return ;;
  esac
  COMPREPLY=($(compgen -W "--json --runtime-url --session --verbose --quiet --help --version" -- "$cur"))
}
complete -F _softprobe softprobe
`

const zshCompletion = `#compdef softprobe
_softprobe() {
  local -a cmds
  cmds=(
    'doctor:check the local environment'
    'session:manage sessions'
    'inspect:read-only inspection'
    'generate:code generation'
    'validate:schema validation'
    'replay:diagnostic'
    'suite:run or validate suites'
    'capture:capture orchestration'
    'scrub:redact case files'
    'export:export to an OTLP endpoint'
    'completion:emit shell completion'
    '--version:print CLI version'
  )
  if (( CURRENT == 2 )); then
    _describe 'softprobe command' cmds
    return
  fi
  case "$words[2]" in
    session)  _values 'session subcommand' start stats close 'load-case' rules policy ;;
    inspect)  _values 'inspect subcommand' case session ;;
    validate) _values 'validate kind' case rules suite ;;
    suite)    _values 'suite subcommand' run validate diff ;;
    *) _files ;;
  esac
}
_softprobe "$@"
`

const fishCompletion = `# fish completion for softprobe
complete -c softprobe -f

set -l top 'doctor session inspect generate validate replay suite capture scrub export completion --version'
complete -c softprobe -n '__fish_use_subcommand' -a "$top"

complete -c softprobe -n '__fish_seen_subcommand_from session' -a 'start stats close load-case rules policy'
complete -c softprobe -n '__fish_seen_subcommand_from inspect' -a 'case session'
complete -c softprobe -n '__fish_seen_subcommand_from validate' -a 'case rules suite'
complete -c softprobe -n '__fish_seen_subcommand_from suite' -a 'run validate diff'

complete -c softprobe -l json -d 'emit structured JSON'
complete -c softprobe -l runtime-url -d 'control runtime base URL'
complete -c softprobe -l session -d 'session ID'
complete -c softprobe -l verbose -d 'extra diagnostic logging'
complete -c softprobe -l quiet -d 'suppress non-error output'
complete -c softprobe -l help -d 'print command help'
complete -c softprobe -l version -d 'print CLI version'
`
