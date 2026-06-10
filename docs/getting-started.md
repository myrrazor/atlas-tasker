# Getting Started

This path gets you from a clean checkout to a usable local Atlas workspace.

## 1. Build The CLI

Until hosted release assets pass the hosted gate, build locally:

```bash
go build -o tracker ./cmd/tracker
./tracker --help
```

If `go build` tries to download a Go toolchain, let it finish or install the pinned Go version from `go.mod`.

## 2. Initialize Atlas

Run this inside the repo or workspace you want Atlas to manage:

```bash
./tracker init
```

Atlas writes local state under `.tracker/`. Keep that directory out of public bug reports unless you have redacted it.

## 3. Create A Project And Ticket

```bash
./tracker project create APP "Example App"
./tracker ticket create --project APP --title "Ship first feature" --type task --actor human:owner --reason "getting started"
./tracker ticket move APP-1 ready --actor human:owner --reason "ready to plan"
```

## 4. Inspect The Work Queue

```bash
./tracker board
./tracker queue --actor human:owner
./tracker inspect APP-1 --actor human:owner
```

Use `--json` when another tool needs structured output:

```bash
./tracker inspect APP-1 --actor human:owner --json
```

## 5. Keep Going

- [Quickstart](quickstart.md) gives one copyable flow.
- [First agent workflow](first-agent-workflow.md) shows the agent run lifecycle.
- [Doctor and repair](guides/doctor-and-repair.md) explains the safe health-check path.
- [Release verification](guides/release-verification.md) explains why a local build is not the same as a hosted release.
