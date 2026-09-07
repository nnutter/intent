package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/util"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/stretchr/testify/require"
)

func newRepo(t *testing.T) *git.Repository {
	t.Helper()
	repo, err := git.Init(memory.NewStorage(), memfs.New())
	require.NoError(t, err)
	return repo
}

func write(t *testing.T, repo *git.Repository, path, content string) {
	t.Helper()
	wt, err := repo.Worktree()
	require.NoError(t, err)
	require.NoError(t, util.WriteFile(wt.Filesystem, path, []byte(content), 0o644))
}

func stage(t *testing.T, repo *git.Repository, paths ...string) {
	t.Helper()
	wt, err := repo.Worktree()
	require.NoError(t, err)
	for _, p := range paths {
		_, err := wt.Add(p)
		require.NoError(t, err)
	}
}

func commit(t *testing.T, repo *git.Repository, msg string) {
	t.Helper()
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add(".")
	require.NoError(t, err)
	_, err = wt.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@example.com", When: time.Now()},
	})
	require.NoError(t, err)
}

func run(t *testing.T, repo *git.Repository, opts Options) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), repo, opts, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func TestRunListsWorktreeRename(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	write(t, repo, "a.go", "package p\nfunc f() { x := 1; _ = x }\n")
	commit(t, repo, "first")
	write(t, repo, "a.go", "package p\nfunc f() { y := 1; _ = y }\n")

	stdout, stderr, err := run(t, repo, Options{})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "scope: worktree")
	require.Contains(t, stdout, "a.go: rename variable x to y in f")
}

func TestRunPrefersStaged(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	write(t, repo, "a.go", "package p\nfunc f() { x := 1; _ = x }\n")
	write(t, repo, "b.go", "package p\nfunc g() { m := 1; _ = m }\n")
	commit(t, repo, "first")
	write(t, repo, "a.go", "package p\nfunc f() { y := 1; _ = y }\n")
	stage(t, repo, "a.go")
	write(t, repo, "b.go", "package p\nfunc g() { n := 1; _ = n }\n")

	stdout, _, err := run(t, repo, Options{})
	require.NoError(t, err)
	require.Contains(t, stdout, "scope: staged")
	require.Contains(t, stdout, "a.go: rename variable x to y in f")
	require.NotContains(t, stdout, "b.go")
}

func TestRunExplicitStagedEmpty(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	write(t, repo, "a.go", "package p\nfunc f() {}\n")
	commit(t, repo, "first")

	stdout, _, err := run(t, repo, Options{Scope: ScopeStaged})
	require.NoError(t, err)
	require.Contains(t, stdout, "no refactorings found in staged changes (0 Go files checked)")
}

func TestRunNoChanges(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	write(t, repo, "a.go", "package p\nfunc f() {}\n")
	commit(t, repo, "first")

	stdout, _, err := run(t, repo, Options{})
	require.NoError(t, err)
	require.Contains(t, stdout, "no refactorings found in worktree changes")
}

func TestRunRejectsBadOptions(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	_, _, err := run(t, repo, Options{Scope: "bogus"})
	require.Error(t, err)
	_, _, err = run(t, repo, Options{Format: "yaml"})
	require.Error(t, err)
}

func TestRunRejectsNilRepo(t *testing.T) {
	t.Parallel()
	_, _, err := run(t, nil, Options{})
	require.Error(t, err)
}

func TestRunJSON(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	write(t, repo, "a.go", "package p\nfunc f() { x := 1; _ = x }\n")
	commit(t, repo, "first")
	write(t, repo, "a.go", "package p\nfunc f() { y := 1; _ = y }\n")
	stage(t, repo, "a.go")

	stdout, _, err := run(t, repo, Options{Format: FormatJSON})
	require.NoError(t, err)
	var report struct {
		Scope    string `json:"scope"`
		Files    int    `json:"files"`
		Findings []struct {
			Kind        string `json:"kind"`
			File        string `json:"file"`
			Description string `json:"description"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &report))
	require.Equal(t, "staged", report.Scope)
	require.Equal(t, 1, report.Files)
	require.Len(t, report.Findings, 1)
	require.Equal(t, "rename-variable", report.Findings[0].Kind)
	require.Equal(t, "a.go", report.Findings[0].File)
	require.Contains(t, report.Findings[0].Description, "x to y")
}

func TestRunWarnsOnUnparseableFile(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	write(t, repo, "bad.go", "package p\nfunc f() {}\n")
	write(t, repo, "good.go", "package p\nfunc f() { x := 1; _ = x }\n")
	commit(t, repo, "first")
	write(t, repo, "bad.go", "package p\nfunc f() {\n")
	write(t, repo, "good.go", "package p\nfunc f() { y := 1; _ = y }\n")

	stdout, stderr, err := run(t, repo, Options{})
	require.NoError(t, err)
	require.Contains(t, stdout, "good.go: rename variable x to y in f")
	require.Contains(t, stderr, "warning")
}

func TestExecuteOnDiskRepoObservesStaged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dir+"/a.go",
		[]byte("package p\nfunc f() { x := 1; _ = x }\n"), 0o644))
	_, err = wt.Add("a.go")
	require.NoError(t, err)
	_, err = wt.Commit("first", &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@example.com", When: time.Now()},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dir+"/a.go",
		[]byte("package p\nfunc f() { y := 1; _ = y }\n"), 0o644))
	_, err = wt.Add("a.go")
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	require.NoError(t, Execute(context.Background(),
		[]string{"--repo", dir}, &stdout, &stderr))
	require.Contains(t, stdout.String(), "scope: staged")
	require.Contains(t, stdout.String(), "rename variable x to y")
}

func TestRootCmdDefaults(t *testing.T) {
	t.Parallel()
	cmd := NewRootCmd()
	require.Equal(t, "intent", cmd.Use)
	require.NoError(t, cmd.ParseFlags([]string{}))
	repo, err := cmd.Flags().GetString("repo")
	require.NoError(t, err)
	require.Equal(t, ".", repo)
	scope, err := cmd.Flags().GetString("scope")
	require.NoError(t, err)
	require.Equal(t, "auto", scope)
	format, err := cmd.Flags().GetString("format")
	require.NoError(t, err)
	require.Equal(t, "text", format)
}
