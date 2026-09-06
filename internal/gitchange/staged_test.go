package gitchange

import (
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/stretchr/testify/require"
)

func stage(t *testing.T, repo *git.Repository, paths ...string) {
	t.Helper()
	wt, err := repo.Worktree()
	require.NoError(t, err)
	for _, p := range paths {
		_, err := wt.Add(p)
		require.NoError(t, err)
	}
}

func TestHasStagedChanges(t *testing.T) {
	t.Parallel()
	require.False(t, HasStagedChanges(nil))
	require.False(t, HasStagedChanges(git.Status{}))
	require.False(t, HasStagedChanges(git.Status{
		"a.go": {Staging: git.Untracked, Worktree: git.Untracked},
	}))
	require.False(t, HasStagedChanges(git.Status{
		"a.go": {Staging: git.Unmodified, Worktree: git.Modified},
	}))
	require.True(t, HasStagedChanges(git.Status{
		"a.go": {Staging: git.Modified, Worktree: git.Unmodified},
	}))
	require.True(t, HasStagedChanges(git.Status{
		"a.go": {Staging: git.Added, Worktree: git.Unmodified},
	}))
}

func TestCheckUnmerged(t *testing.T) {
	t.Parallel()
	require.NoError(t, checkUnmerged(git.Status{
		"a.go": {Staging: git.Modified, Worktree: git.Unmodified},
	}))
	require.Error(t, checkUnmerged(git.Status{
		"a.go": {Staging: git.UpdatedButUnmerged, Worktree: git.Unmodified},
	}))
	require.Error(t, checkUnmerged(git.Status{
		"a.go": {Staging: git.Unmodified, Worktree: git.UpdatedButUnmerged},
	}))
}

func TestStagedChangesModified(t *testing.T) {
	t.Parallel()
	repo := initMemRepo(t)
	writeFile(t, repo, "a.go", "package p\nfunc f() {}\n")
	commitAll(t, repo, "first")
	writeFile(t, repo, "a.go", "package p\nfunc f() { println(1) }\n")
	stage(t, repo, "a.go")

	changes, err := StagedChanges(repo)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, "a.go", changes[0].Path)
	require.Contains(t, string(changes[0].Before), "func f() {}")
	require.Contains(t, string(changes[0].After), "println")
}

func TestStagedChangesReadsIndexNotWorktree(t *testing.T) {
	t.Parallel()
	repo := initMemRepo(t)
	writeFile(t, repo, "a.go", "package p\nfunc f() {}\n")
	commitAll(t, repo, "first")
	writeFile(t, repo, "a.go", "package p\nfunc f() { println(\"staged\") }\n")
	stage(t, repo, "a.go")
	writeFile(t, repo, "a.go", "package p\nfunc f() { println(\"unstaged\") }\n")

	changes, err := StagedChanges(repo)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Contains(t, string(changes[0].After), "staged")
	require.NotContains(t, string(changes[0].After), "unstaged")
}

func TestStagedChangesAddedWithoutHead(t *testing.T) {
	t.Parallel()
	repo := initMemRepo(t)
	writeFile(t, repo, "new.go", "package p\nfunc g() {}\n")
	stage(t, repo, "new.go")

	changes, err := StagedChanges(repo)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, "new.go", changes[0].Path)
	require.Nil(t, changes[0].Before)
	require.NotNil(t, changes[0].After)
}

func TestStagedChangesDeleted(t *testing.T) {
	t.Parallel()
	repo := initMemRepo(t)
	writeFile(t, repo, "a.go", "package p\nfunc f() {}\n")
	writeFile(t, repo, "b.go", "package p\nfunc g() {}\n")
	commitAll(t, repo, "first")
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Remove("a.go")
	require.NoError(t, err)

	changes, err := StagedChanges(repo)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, "a.go", changes[0].Path)
	require.NotNil(t, changes[0].Before)
	require.Nil(t, changes[0].After)
}

func TestStagedChangesEmpty(t *testing.T) {
	t.Parallel()
	repo := initMemRepo(t)
	writeFile(t, repo, "a.go", "package p\nfunc f() {}\n")
	commitAll(t, repo, "first")

	changes, err := StagedChanges(repo)
	require.NoError(t, err)
	require.Empty(t, changes)
}

func TestStagedChangesRejectsUnmergedIndex(t *testing.T) {
	t.Parallel()
	repo := initMemRepo(t)
	writeFile(t, repo, "a.go", "package p\nfunc f() {}\n")
	commitAll(t, repo, "first")

	idx, err := repo.Storer.Index()
	require.NoError(t, err)
	idx.Entries = append(idx.Entries, &index.Entry{Name: "a.go", Stage: 1})
	require.NoError(t, repo.Storer.SetIndex(idx))

	_, err = StagedChanges(repo)
	require.Error(t, err)
}

func TestObserveChangesPrefersStaged(t *testing.T) {
	t.Parallel()
	repo := initMemRepo(t)
	writeFile(t, repo, "a.go", "package p\nfunc f() {}\n")
	writeFile(t, repo, "b.go", "package p\nfunc g() {}\n")
	commitAll(t, repo, "first")
	writeFile(t, repo, "a.go", "package p\nfunc f() { println(1) }\n")
	stage(t, repo, "a.go")
	writeFile(t, repo, "b.go", "package p\nfunc g() { println(2) }\n")

	changes, scope, err := ObserveChanges(repo)
	require.NoError(t, err)
	require.Equal(t, ScopeStaged, scope)
	require.Len(t, changes, 1)
	require.Equal(t, "a.go", changes[0].Path)
}

func TestObserveChangesFallsBackToWorktree(t *testing.T) {
	t.Parallel()
	repo := initMemRepo(t)
	writeFile(t, repo, "a.go", "package p\nfunc f() {}\n")
	commitAll(t, repo, "first")
	writeFile(t, repo, "a.go", "package p\nfunc f() { println(1) }\n")

	changes, scope, err := ObserveChanges(repo)
	require.NoError(t, err)
	require.Equal(t, ScopeWorktree, scope)
	require.Len(t, changes, 1)
}

func TestObserveChangesNonGoStagingStillLimitsScope(t *testing.T) {
	t.Parallel()
	repo := initMemRepo(t)
	writeFile(t, repo, "a.go", "package p\nfunc f() {}\n")
	writeFile(t, repo, "notes.md", "hi\n")
	commitAll(t, repo, "first")
	writeFile(t, repo, "notes.md", "hi there\n")
	stage(t, repo, "notes.md")
	writeFile(t, repo, "a.go", "package p\nfunc f() { println(1) }\n")

	changes, scope, err := ObserveChanges(repo)
	require.NoError(t, err)
	require.Equal(t, ScopeStaged, scope)
	require.Empty(t, changes)
}
