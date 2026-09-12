<h1 align="center">
  tools
</h1>

> A small personal Go CLI that runs named subcommands, kubectl-style:
> `tools <subcommand> [args...]`.

`tools` is a minimal dispatcher binary. `main.go` holds a table of
subcommand names to handlers; each subcommand's real logic lives in its
own `internal/<name>` package. There is no plugin system, no config file,
and no discovery mechanism — adding a subcommand means adding an entry to
the dispatch table and a package under `internal/`.

## Install

Download and run the installer from GitHub with `curl`:

```bash
curl -fsSL https://raw.githubusercontent.com/bruaguspons/tools/main/install.sh | bash
```

This downloads the latest GitHub Release for your platform (`linux/amd64`
or `linux/arm64`) and installs the `tools` binary to `~/.local/bin`.
Re-running the same command updates an existing install to the latest
release. The installer only ever manages the binary itself — there is no
bundled snapshot data to install alongside it.

## Usage

```bash
tools                # print usage (lists registered subcommands) and exit non-zero
tools -version       # print version and exit
tools -h | --help    # print usage and exit 0
tools <subcommand>   # run a registered subcommand
```

Running with no subcommand, or with an unrecognized one, prints usage to
stderr and exits non-zero (`2`). A subcommand that runs but fails exits
`1`.

### `vscode-java`

```bash
tools vscode-java
```

Merges the VS Code Java extension's run/debug settings into
`./.vscode/settings.json` in the current directory:

- Reads the JDK path from the `JAVA_HOME` environment variable. There is
  no filesystem discovery fallback — if `JAVA_HOME` is unset, empty, or
  doesn't contain `bin/java`, the command fails loudly and makes no
  changes.
- Creates `.vscode/` and `settings.json` if they don't exist yet.
- If `settings.json` already exists, it is parsed as **strict JSON**. VS
  Code itself tolerates JSONC (`//`/`/*` comments, trailing commas), but
  this tool does not: a file containing either fails with a clear error
  and is left untouched, rather than silently stripping comments on
  write.
- Merges exactly two owned keys, preserving every other key and the
  original key order:
  - `java.jdt.ls.java.home` — set to `$JAVA_HOME`.
  - `java.configuration.updateBuildConfiguration` — set to `"automatic"`.
- The write is atomic (staged in a temp file inside `.vscode/`, then
  renamed into place) and skipped entirely if the merged content would be
  byte-identical to what's already on disk — running the command twice
  with the same `JAVA_HOME` is a no-op the second time.

## Contributing

```bash
go build .              # build the tools binary
go test ./...           # run all Go tests
go test ./internal/...  # test a single package
bash install_test.sh    # test the installer script
gofmt -l .              # check formatting (no golangci-lint config in this repo)
go vet ./...
```

To add a new subcommand: add an `internal/<name>` package with the real
logic and its tests, then register it in `main.go`'s `commands` map.

## Migration note

This repository previously hosted a different, more complex CLI built
around a provider/diff/lock sync abstraction for tool versions and
Claude Code skills. That entire domain has been removed in favor of this
minimal dispatcher. If you have an old install from that earlier tool,
its binary and any leftover per-user data directory are **not** cleaned
up automatically by this change — remove them by hand if you no longer
need them.
