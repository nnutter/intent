# intent

Static analysis to find Go refactorings in uncommitted or committed changes,
so refactorings can later be split out from functional changes.

MVP detects five local refactorings with `go-git` (no `git` CLI):

- rename variable (including params)
- extract variable / inline variable
- extract function / inline function

## CLI

```sh
go run .                      # auto scope, text output
go run . --scope staged      # HEAD versus the index only
go run . --scope worktree    # HEAD versus the worktree even if staged
go run . --commit HEAD       # that commit versus its first parent
go run . --commit abc123
go run . --format json       # machine-readable report
go run . --repo /path/to/repo
```

Scope rule: when staged changes are present only the staged set is
observed; otherwise the worktree set is observed. Pass `--commit <rev>`
to observe a commit versus its parent instead. The observed scope is
always printed so reports cannot be mistaken for each other.
Each finding is `file:line: description` using a 1-based line in the
after source.

Safety: the CLI only reads the repository (status, HEAD, index blobs,
worktree files, commit trees). It never stages, commits, resets, or
edits anything, so a failed run leaves the repo untouched. Repositories
with conflict markers are refused instead of half-reported. Linked
worktrees are opened with git `commondir` support so HEAD and objects
resolve from the shared repository.

## Layout

- `internal/refactor`: pure detection on `FileChange{Path, Before, After}`.
  `Detect` handles one file, `DetectChanges` runs many files concurrently.
- `internal/gitchange`: `WorktreeChanges` (HEAD vs worktree),
  `StagedChanges` (HEAD vs index), `ObserveChanges` (staged when any
  are staged, else worktree), and `CommitChanges` (commit vs parent).
- `internal/cli`: cobra/fang command; `Run` takes an opened repo plus
  writers so tests stay hermetic. `main.go` only wires signals and exit
  codes.

Only `go-billy` (via `go-git`) is used for filesystems. Tests use
`memfs.New()` + `memory.NewStorage()` for hermetic in-memory repos.

## Example

```go
changes, _ := gitchange.WorktreeChanges(repo)
findings, _ := refactor.DetectChanges(changes)
for _, r := range findings {
    fmt.Printf("%s:%d: %s\n", r.Path(), r.Line(), r.Describe())
}
```

## Limits (MVP)

- Same-file, locals only; no cross-file moves.
- Expression/function matching is syntactic, not `go/types` precise.
- Trivial `x` / `42` variable extracts are ignored to reduce noise.
- Param-renamed function extracts may miss.

## Checks

```sh
go test -race -shuffle=on ./...
go vet ./...
```

`go mod tidy` uses `-e` because one pinned tool (`constable`) is private
and otherwise breaks tidy on machines without its SSH access.
