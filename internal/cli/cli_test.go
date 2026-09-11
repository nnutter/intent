package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/util"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
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

func commit(t *testing.T, repo *git.Repository, msg string) plumbing.Hash {
	t.Helper()
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add(".")
	require.NoError(t, err)
	h, err := wt.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@example.com", When: time.Now()},
	})
	require.NoError(t, err)
	return h
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
	require.Contains(t, stdout, "a.go:2: rename variable x to y in f")
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
	require.Contains(t, stdout, "a.go:2: rename variable x to y in f")
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
	_, _, err = run(t, repo, Options{Scope: ScopeStaged, Commit: "HEAD"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--commit cannot be combined")
}

func TestRunCommitIgnoresWorktree(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	write(t, repo, "a.go", "package p\nfunc f() { x := 1; _ = x }\n")
	commit(t, repo, "first")
	write(t, repo, "a.go", "package p\nfunc f() { y := 1; _ = y }\n")
	second := commit(t, repo, "second")
	write(t, repo, "a.go", "package p\nfunc f() { z := 1; _ = z }\n")

	stdout, stderr, err := run(t, repo, Options{Commit: second.String()})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "scope: commit "+second.String())
	require.Contains(t, stdout, "a.go:2: rename variable x to y in f")
	require.NotContains(t, stdout, "z")
}

func TestRunCommitHEAD(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	write(t, repo, "a.go", "package p\nfunc f() { x := 1; _ = x }\n")
	commit(t, repo, "first")
	write(t, repo, "a.go", "package p\nfunc f() { y := 1; _ = y }\n")
	head := commit(t, repo, "second")
	write(t, repo, "a.go", "package p\nfunc f() { z := 1; _ = z }\n")
	stage(t, repo, "a.go")

	stdout, _, err := run(t, repo, Options{Commit: "HEAD"})
	require.NoError(t, err)
	require.Contains(t, stdout, "scope: commit "+head.String())
	require.Contains(t, stdout, "rename variable x to y")
	require.NotContains(t, stdout, "z")
}

func TestRunCommitUnknownRevision(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	write(t, repo, "a.go", "package p\nfunc f() {}\n")
	commit(t, repo, "first")

	_, _, err := run(t, repo, Options{Commit: "not-a-commit"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not-a-commit")
}

func TestRunCommitJSON(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	write(t, repo, "a.go", "package p\nfunc f() { x := 1; _ = x }\n")
	commit(t, repo, "first")
	write(t, repo, "a.go", "package p\nfunc f() { y := 1; _ = y }\n")
	second := commit(t, repo, "second")

	stdout, _, err := run(t, repo, Options{Commit: "HEAD", Format: FormatJSON})
	require.NoError(t, err)
	var report struct {
		Scope    string `json:"scope"`
		Commit   string `json:"commit"`
		Files    int    `json:"files"`
		Findings []struct {
			Kind        string `json:"kind"`
			File        string `json:"file"`
			Line        int    `json:"line"`
			Description string `json:"description"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &report))
	require.Equal(t, "commit", report.Scope)
	require.Equal(t, second.String(), report.Commit)
	require.Equal(t, 1, report.Files)
	require.Len(t, report.Findings, 1)
	require.Equal(t, 2, report.Findings[0].Line)
	require.Contains(t, report.Findings[0].Description, "x to y")
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
			Line        int    `json:"line"`
			Description string `json:"description"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &report))
	require.Equal(t, "staged", report.Scope)
	require.Equal(t, 1, report.Files)
	require.Len(t, report.Findings, 1)
	require.Equal(t, "rename-variable", report.Findings[0].Kind)
	require.Equal(t, "a.go", report.Findings[0].File)
	require.Equal(t, 2, report.Findings[0].Line)
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
	require.Contains(t, stdout, "good.go:2: rename variable x to y in f")
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
		[]string{"--repo", dir}, &stdout, &stderr, "dev"))
	require.Contains(t, stdout.String(), "scope: staged")
	require.Contains(t, stdout.String(), "rename variable x to y")
}

func TestExecuteOnDiskRepoObservesCommit(t *testing.T) {
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
	head, err := wt.Commit("second", &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@example.com", When: time.Now()},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dir+"/a.go",
		[]byte("package p\nfunc f() { z := 1; _ = z }\n"), 0o644))

	var stdout, stderr bytes.Buffer
	require.NoError(t, Execute(context.Background(),
		[]string{"--repo", dir, "--commit", "HEAD"}, &stdout, &stderr, "dev"))
	require.Contains(t, stdout.String(), "scope: commit "+head.String())
	require.Contains(t, stdout.String(), "rename variable x to y")
	require.NotContains(t, stdout.String(), "z")
}

func TestExecuteLinkedWorktreeResolvesHEAD(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := t.TempDir()
	mainDir := filepath.Join(root, "main")
	linkDir := filepath.Join(root, "linked")
	require.NoError(t, os.Mkdir(mainDir, 0o755))

	runGit(t, mainDir, "init", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "a.go"),
		[]byte("package p\nfunc f() { x := 1; _ = x }\n"), 0o644))
	runGit(t, mainDir, "add", "a.go")
	runGit(t, mainDir, "commit", "-m", "first")
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "a.go"),
		[]byte("package p\nfunc f() { y := 1; _ = y }\n"), 0o644))
	runGit(t, mainDir, "add", "a.go")
	runGit(t, mainDir, "commit", "-m", "second")
	runGit(t, mainDir, "worktree", "add", "-b", "linked", linkDir)

	var stdout, stderr bytes.Buffer
	require.NoError(t, Execute(context.Background(),
		[]string{"--repo", linkDir, "--commit", "HEAD"}, &stdout, &stderr, "dev"))
	require.Empty(t, stderr.String())
	require.Contains(t, stdout.String(), "scope: commit")
	require.Contains(t, stdout.String(), "rename variable x to y")

	stdout.Reset()
	stderr.Reset()
	require.NoError(t, Execute(context.Background(),
		[]string{"--repo", linkDir}, &stdout, &stderr, "dev"))
	require.Empty(t, stderr.String())
	require.Contains(t, stdout.String(), "no refactorings found in worktree changes")
}

func TestExecuteDetectsDotGitFromSubdir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"),
		[]byte("package p\nfunc f() { x := 1; _ = x }\n"), 0o644))
	_, err = wt.Add("a.go")
	require.NoError(t, err)
	_, err = wt.Commit("first", &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@example.com", When: time.Now()},
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	require.NoError(t, Execute(context.Background(),
		[]string{"--repo", filepath.Join(dir, "sub")}, &stdout, &stderr, "dev"))
	require.Empty(t, stderr.String())
	require.Contains(t, stdout.String(), "no refactorings found")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t",
		"GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
}

func TestRootCmdVersion(t *testing.T) {
	t.Parallel()
	cmd := NewRootCmd("v1.2.3")
	require.Equal(t, "v1.2.3", cmd.Version)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"--version"})
	require.NoError(t, cmd.Execute())
	require.Contains(t, stdout.String(), "v1.2.3")
}

func TestRootCmdDefaults(t *testing.T) {
	t.Parallel()
	cmd := NewRootCmd("dev")
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
	commitFlag, err := cmd.Flags().GetString("commit")
	require.NoError(t, err)
	require.Empty(t, commitFlag)
}
