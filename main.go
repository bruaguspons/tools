// Command tools is a small personal CLI that runs named subcommands,
// kubectl-style: `tools <subcommand> [args...]`.
package main

import (
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/bruaguspons/tools/internal/vscodejava"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

// command is one entry in the subcommand dispatch table.
type command struct {
	summary string
	run     func(args []string, stdout, stderr io.Writer) int
}

var commands = map[string]command{
	"vscode-java": {
		summary: "merge Java run/debug settings into ./.vscode/settings.json",
		run:     runVscodeJava,
	},
}

func runVscodeJava(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintf(stderr, "tools: vscode-java takes no arguments (got %v)\n", args)
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "tools: %v\n", err)
		return 1
	}

	result, err := vscodejava.Configure(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "tools: %v\n", err)
		return 1
	}

	if result.Changed {
		fmt.Fprintf(stdout, "tools: updated %s\n", result.Path)
	} else {
		fmt.Fprintf(stdout, "tools: %s already up to date\n", result.Path)
	}
	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: tools <subcommand> [args...]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Subcommands:")

	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(w, "  %-16s %s\n", name, commands[name].summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  -version         print the tools version and exit")
	fmt.Fprintln(w, "  -h, --help       show this usage text")
}

// run dispatches args (os.Args[1:]) to the matching subcommand and returns
// the process exit code: 0 ok, 1 command error, 2 usage error (missing or
// unknown subcommand, or unexpected arguments).
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "-version", "--version":
		fmt.Fprintln(stdout, "tools "+version)
		return 0
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	}

	name := args[0]
	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(stderr, "tools: %q is not a known subcommand\n\n", name)
		printUsage(stderr)
		return 2
	}

	return cmd.run(args[1:], stdout, stderr)
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
