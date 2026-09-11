package gitchange

import (
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/util"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/stretchr/testify/require"
)

func initMemRepo(t *testing.T) *git.Repository {
	t.Helper()
	repo, err := git.Init(memory.NewStorage(), memfs.New())
	require.NoError(t, err)
	return repo
}

func commitAll(
	t *testing.T,
	repo *git.Repository,
	msg string,
) object.Commit {
	t.Helper()
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add(".")
	require.NoError(t, err)
	h, err := wt.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@example.com", When: time.Now()},
	})
	require.NoError(t, err)
	c, err := repo.CommitObject(h)
	require.NoError(t, err)
	return *c
}

func writeFile(t *testing.T, repo *git.Repository, path, content string) {
	t.Helper()
	wt, err := repo.Worktree()
	require.NoError(t, err)
	require.NoError(t, util.WriteFile(wt.Filesystem, path, []byte(content), 0o644))
}

func TestWorktreeChangesModified(t *testing.T) {
	t.Parallel()
	repo := initMemRepo(t)
	writeFile(t, repo, "a.go", "package p\nfunc f() {}\n")
	commitAll(t, repo, "first")
	writeFile(t, repo, "a.go", "package p\nfunc f() { println(1) }\n")

	changes, err := WorktreeChanges(repo)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, "a.go", changes[0].Path)
	require.Contains(t, string(changes[0].Before), "func f() {}")
	require.Contains(t, string(changes[0].After), "println")
}

func TestWorktreeChangesIgnoresNonGo(t *testing.T) {
	t.Parallel()
	repo := initMemRepo(t)
	writeFile(t, repo, "a.go", "package p\nfunc f() {}\n")
	commitAll(t, repo, "first")
	writeFile(t, repo, "README.md", "hello\n")

	changes, err := WorktreeChanges(repo)
	require.NoError(t, err)
	require.Empty(t, changes)
}

func TestWorktreeChangesUntracked(t *testing.T) {
	t.Parallel()
	repo := initMemRepo(t)
	writeFile(t, repo, "new.go", "package p\nfunc g() {}\n")

	changes, err := WorktreeChanges(repo)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, "new.go", changes[0].Path)
	require.Nil(t, changes[0].Before)
	require.NotNil(t, changes[0].After)
}

func TestCommitChanges(t *testing.T) {
	t.Parallel()
	repo := initMemRepo(t)
	writeFile(t, repo, "a.go", "package p\nfunc f() {}\n")
	commitAll(t, repo, "first")
	writeFile(t, repo, "a.go", "package p\nfunc f() { println(1) }\n")
	second := commitAll(t, repo, "second")

	changes, err := CommitChanges(repo, second.Hash)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, "a.go", changes[0].Path)
	require.Contains(t, string(changes[0].Before), "func f() {}")
	require.Contains(t, string(changes[0].After), "println")
}

func TestCommitChangesRoot(t *testing.T) {
	t.Parallel()
	repo := initMemRepo(t)
	writeFile(t, repo, "a.go", "package p\nfunc f() {}\n")
	first := commitAll(t, repo, "first")

	changes, err := CommitChanges(repo, first.Hash)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Nil(t, changes[0].Before)
	require.NotNil(t, changes[0].After)
}

func TestResolveCommit(t *testing.T) {
	t.Parallel()
	repo := initMemRepo(t)
	writeFile(t, repo, "a.go", "package p\nfunc f() {}\n")
	first := commitAll(t, repo, "first")

	h, err := ResolveCommit(repo, "HEAD")
	require.NoError(t, err)
	require.Equal(t, first.Hash, h)

	h, err = ResolveCommit(repo, first.Hash.String())
	require.NoError(t, err)
	require.Equal(t, first.Hash, h)

	h, err = ResolveCommit(repo, first.Hash.String()[:12])
	require.NoError(t, err)
	require.Equal(t, first.Hash, h)

	_, err = ResolveCommit(repo, "")
	require.Error(t, err)
	_, err = ResolveCommit(nil, "HEAD")
	require.Error(t, err)
	_, err = ResolveCommit(repo, "missing")
	require.Error(t, err)
}
