// Package gitchange loads changed Go files via go-git without shelling out.
//
// It works with both on-disk repositories and in-memory repositories
// (memory.Storage with billy memfs), which keeps tests hermetic.
package gitchange

import (
	"bytes"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/go-git/go-billy/v5/util"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/nnutter/intent/internal/refactor"
)

//constable:nonmutating
func isGoFile(path string) bool {
	return strings.HasSuffix(path, ".go")
}

// WorktreeChanges returns Go file changes between HEAD and the worktree.
// Untracked files have nil Before; deleted files have nil After.
// Unmodified or content-identical files are skipped.
//
//constable:nonmutating
func WorktreeChanges(repo *git.Repository) ([]refactor.FileChange, error) {
	if repo == nil {
		return nil, nil
	}
	wt, status, err := worktreeStatus(repo)
	if err != nil {
		return nil, err
	}
	headCommit, err := loadHead(repo)
	if err != nil {
		return nil, err
	}
	return worktreeChangesFrom(wt, status, headCommit)
}

// worktreeStatus opens the worktree and reads its status once so callers
// observing large repositories pay for a single worktree walk.
//
//constable:nonmutating
func worktreeStatus(repo *git.Repository) (*git.Worktree, git.Status, error) {
	wt, err := repo.Worktree()
	if err != nil {
		return nil, nil, err
	}
	status, err := wt.Status()
	if err != nil {
		return nil, nil, err
	}
	return wt, status, nil
}

//constable:nonmutating
func worktreeChangesFrom(
	wt *git.Worktree,
	status git.Status,
	headCommit *object.Commit,
) ([]refactor.FileChange, error) {
	var out []refactor.FileChange
	for path := range status {
		if !isGoFile(path) {
			continue
		}
		after, err := readWorktreeFile(wt, path)
		if err != nil {
			return nil, err
		}
		var before []byte
		if headCommit != nil {
			b, err := readHeadFile(headCommit, path)
			if err != nil {
				return nil, err
			}
			before = b
		}
		if before == nil && after == nil {
			continue
		}
		if len(before) > 0 && len(after) > 0 && bytes.Equal(before, after) {
			continue
		}
		out = append(out, refactor.FileChange{Path: path, Before: before, After: after})
	}
	slices.SortFunc(out, func(a, b refactor.FileChange) int {
		return strings.Compare(a.Path, b.Path)
	})
	return out, nil
}

// loadHead returns the HEAD commit, or nil when the repository has no
// commits yet. Unexpected lookup failures are returned so callers fail
// closed instead of silently observing against an empty base.
//
//constable:nonmutating
func loadHead(repo *git.Repository) (*object.Commit, error) {
	ref, err := repo.Head()
	if err != nil {
		if err == plumbing.ErrReferenceNotFound {
			return nil, nil
		}
		return nil, err
	}
	return repo.CommitObject(ref.Hash())
}

//constable:nonmutating
func readWorktreeFile(wt *git.Worktree, path string) ([]byte, error) {
	data, err := util.ReadFile(wt.Filesystem, path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

//constable:nonmutating
func readHeadFile(commit *object.Commit, path string) ([]byte, error) {
	f, err := commit.File(path)
	if err != nil {
		if err == object.ErrFileNotFound {
			return nil, nil
		}
		return nil, err
	}
	content, err := f.Contents()
	if err != nil {
		return nil, err
	}
	return []byte(content), nil
}

// CommitChanges returns Go file changes in one commit versus its first
// parent. A root commit reports all its Go files as added.
//
//constable:nonmutating
func CommitChanges(repo *git.Repository, hash plumbing.Hash) ([]refactor.FileChange, error) {
	if repo == nil {
		return nil, nil
	}
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return nil, err
	}
	parent, err := firstParent(commit)
	if err != nil {
		return nil, err
	}
	if parent == nil {
		return rootCommitChanges(commit)
	}
	parentTree, err := parent.Tree()
	if err != nil {
		return nil, err
	}
	commitTree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	changes, err := parentTree.Diff(commitTree)
	if err != nil {
		return nil, err
	}
	var out []refactor.FileChange
	for _, ch := range changes {
		fc, ok, err := fileChangeFromDiff(ch)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out = append(out, fc)
	}
	slices.SortFunc(out, func(a, b refactor.FileChange) int {
		return strings.Compare(a.Path, b.Path)
	})
	return out, nil
}

//constable:nonmutating
func firstParent(commit *object.Commit) (*object.Commit, error) {
	iter := commit.Parents()
	defer iter.Close()
	parent, err := iter.Next()
	if err != nil {
		if err == io.EOF {
			return nil, nil
		}
		return nil, err
	}
	return parent, nil
}

//constable:nonmutating
func rootCommitChanges(commit *object.Commit) ([]refactor.FileChange, error) {
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	iter := tree.Files()
	defer iter.Close()
	var out []refactor.FileChange
	for {
		f, err := iter.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if !isGoFile(f.Name) {
			continue
		}
		content, err := f.Contents()
		if err != nil {
			return nil, err
		}
		out = append(out, refactor.FileChange{Path: f.Name, After: []byte(content)})
	}
	slices.SortFunc(out, func(a, b refactor.FileChange) int {
		return strings.Compare(a.Path, b.Path)
	})
	return out, nil
}

//constable:nonmutating
func fileChangeFromDiff(ch *object.Change) (refactor.FileChange, bool, error) {
	from, to, err := ch.Files()
	if err != nil {
		return refactor.FileChange{}, false, err
	}
	if from == nil && to == nil {
		return refactor.FileChange{}, false, nil
	}
	var fc refactor.FileChange
	if to != nil {
		fc.Path = ch.To.Name
		if !isGoFile(fc.Path) {
			return refactor.FileChange{}, false, nil
		}
		content, err := to.Contents()
		if err != nil {
			return refactor.FileChange{}, false, err
		}
		fc.After = []byte(content)
	} else {
		fc.Path = ch.From.Name
		if !isGoFile(fc.Path) {
			return refactor.FileChange{}, false, nil
		}
	}
	if from != nil {
		content, err := from.Contents()
		if err != nil {
			return refactor.FileChange{}, false, err
		}
		fc.Before = []byte(content)
	}
	if len(fc.Before) > 0 && len(fc.After) > 0 && bytes.Equal(fc.Before, fc.After) {
		return refactor.FileChange{}, false, nil
	}
	return fc, true, nil
}
