package gitchange

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/nnutter/intent/internal/refactor"
)

// Scope names which change set was observed.
type Scope string

// Supported observation scopes.
const (
	// ScopeStaged is HEAD versus the index.
	ScopeStaged Scope = "staged"
	// ScopeWorktree is HEAD versus the worktree.
	ScopeWorktree Scope = "worktree"
	// ScopeCommit is one commit versus its first parent.
	ScopeCommit Scope = "commit"
)

// HasStagedChanges reports whether any path has a staged entry.
// Untracked files are not staged.
//
//constable:nonmutating
func HasStagedChanges(status git.Status) bool {
	for _, fs := range status {
		if fs == nil {
			continue
		}
		switch fs.Staging {
		case git.Unmodified, git.Untracked:
			continue
		default:
			return true
		}
	}
	return false
}

// checkUnmerged refuses to observe repositories with conflict markers so
// callers fail with a clear message instead of reporting on halves.
//
//constable:nonmutating
func checkUnmerged(status git.Status) error {
	for path, fs := range status {
		if fs == nil {
			continue
		}
		if fs.Staging == git.UpdatedButUnmerged || fs.Worktree == git.UpdatedButUnmerged {
			return fmt.Errorf("unmerged path %q: resolve conflicts before observing", path)
		}
	}
	return nil
}

// ObserveChanges implements the CLI scope rule with a single status walk
// for performance: when staged changes are present only the staged set
// (HEAD versus the index) is returned, otherwise the worktree set.
//
// It is read-only: it never stages, commits, or modifies files, so a
// failure partway leaves the repository exactly as found.
//
//constable:nonmutating
func ObserveChanges(repo *git.Repository) ([]refactor.FileChange, Scope, error) {
	if repo == nil {
		return nil, "", nil
	}
	wt, status, err := worktreeStatus(repo)
	if err != nil {
		return nil, "", err
	}
	if err := checkUnmerged(status); err != nil {
		return nil, "", err
	}
	head, err := loadHead(repo)
	if err != nil {
		return nil, "", err
	}
	if HasStagedChanges(status) {
		idx, err := repo.Storer.Index()
		if err != nil {
			return nil, "", err
		}
		changes, err := stagedChangesFrom(status, head, idx, repo)
		if err != nil {
			return nil, "", err
		}
		return changes, ScopeStaged, nil
	}
	changes, err := worktreeChangesFrom(wt, status, head)
	if err != nil {
		return nil, "", err
	}
	return changes, ScopeWorktree, nil
}

// StagedChanges returns Go file changes between HEAD and the index.
// Files staged as deleted have nil After; files with no HEAD version
// have nil Before.
//
//constable:nonmutating
func StagedChanges(repo *git.Repository) ([]refactor.FileChange, error) {
	if repo == nil {
		return nil, nil
	}
	_, status, err := worktreeStatus(repo)
	if err != nil {
		return nil, err
	}
	if err := checkUnmerged(status); err != nil {
		return nil, err
	}
	head, err := loadHead(repo)
	if err != nil {
		return nil, err
	}
	idx, err := repo.Storer.Index()
	if err != nil {
		return nil, err
	}
	return stagedChangesFrom(status, head, idx, repo)
}

//constable:nonmutating
func stagedChangesFrom(
	status git.Status,
	head *object.Commit,
	idx *index.Index,
	repo *git.Repository,
) ([]refactor.FileChange, error) {
	byName := make(map[string]*index.Entry, len(idx.Entries))
	for _, e := range idx.Entries {
		if e == nil {
			continue
		}
		if e.Stage != 0 {
			return nil, fmt.Errorf("unmerged path %q: resolve conflicts before observing", e.Name)
		}
		byName[e.Name] = e
	}
	var out []refactor.FileChange
	for path, fs := range status {
		if !isGoFile(path) || fs == nil {
			continue
		}
		switch fs.Staging {
		case git.Deleted:
			before, err := readHeadFileNil(head, path)
			if err != nil {
				return nil, err
			}
			if before == nil {
				continue
			}
			out = append(out, refactor.FileChange{Path: path, After: nil, Before: before})
		case git.Modified, git.Added, git.Copied:
			entry, ok := byName[path]
			if !ok || entry.IntentToAdd {
				// No staged content (intent-to-add records only the
				// future path, like `git diff --cached` showing nothing).
				continue
			}
			if !isStagedFile(entry.Mode) {
				continue
			}
			after, err := readStagedBlob(repo, entry.Hash)
			if err != nil {
				return nil, fmt.Errorf("read staged %q: %w", path, err)
			}
			before, err := readHeadFileNil(head, path)
			if err != nil {
				return nil, err
			}
			if len(before) > 0 && len(after) > 0 && bytes.Equal(before, after) {
				continue
			}
			out = append(out, refactor.FileChange{Path: path, Before: before, After: after})
		default:
			// Unmodified and untracked paths have no staged content.
			continue
		}
	}
	slices.SortFunc(out, func(a, b refactor.FileChange) int {
		return strings.Compare(a.Path, b.Path)
	})
	return out, nil
}

// isStagedFile reports whether an index entry is a regular file worth
// parsing. Symlinks, submodules, and directories are skipped.
//
//constable:nonmutating
func isStagedFile(mode filemode.FileMode) bool {
	return mode == filemode.Regular ||
		mode == filemode.Deprecated ||
		mode == filemode.Executable
}

//constable:nonmutating
func readHeadFileNil(head *object.Commit, path string) ([]byte, error) {
	if head == nil {
		return nil, nil
	}
	return readHeadFile(head, path)
}

//constable:nonmutating
func readStagedBlob(repo *git.Repository, hash plumbing.Hash) ([]byte, error) {
	blob, err := repo.BlobObject(hash)
	if err != nil {
		return nil, err
	}
	r, err := blob.Reader()
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return io.ReadAll(r)
}
