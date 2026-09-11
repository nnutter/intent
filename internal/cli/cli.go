// Package cli wires refactoring detection to a read-only command.
//
// SAFETY: every code path here only reads the repository (status, HEAD,
// index blobs, worktree files). Nothing stages, commits, resets, or
// writes, so a failure partway leaves the repository exactly as found.
// Errors abort before any findings print, so partial results are never
// presented as complete.
package cli

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"

	charmfang "charm.land/fang/v2"
	"github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"

	"github.com/nnutter/intent/internal/gitchange"
	"github.com/nnutter/intent/internal/refactor"
)

// Scope options for which changes to observe.
const (
	ScopeAuto     = "auto"
	ScopeStaged   = "staged"
	ScopeWorktree = "worktree"
)

// Format options for output.
const (
	FormatText = "text"
	FormatJSON = "json"
)

// Options configures one observation run.
type Options struct {
	// Scope selects auto, staged, or worktree. Empty means auto.
	Scope string
	// Format selects text or json. Empty means text.
	Format string
}

//constable:nonmutating
func (o Options) format() string {
	return cmp.Or(o.Format, FormatText)
}

//constable:nonmutating
func (o Options) scope() string {
	return cmp.Or(o.Scope, ScopeAuto)
}

// Run observes changes, detects refactorings, and writes the report.
// It never modifies the repository.
func Run(
	ctx context.Context,
	repo *git.Repository,
	opts Options,
	stdout, stderr io.Writer,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if repo == nil {
		return fmt.Errorf("nil repository")
	}
	if scope, format := opts.scope(), opts.format(); !validScope(scope) || !validFormat(format) {
		return fmt.Errorf("invalid options: scope must be auto|staged|worktree, format text|json")
	}
	changes, scope, err := observe(repo, opts.scope())
	if err != nil {
		return err
	}
	findings, detectErr := refactor.DetectChanges(changes)
	if err := writeReport(stdout, opts.format(), scope, len(changes), findings); err != nil {
		return err
	}
	if detectErr != nil {
		if _, err := fmt.Fprintf(stderr, "warning: some files were skipped: %v\n", detectErr); err != nil {
			return err
		}
	}
	return nil
}

//constable:nonmutating
func validScope(s string) bool {
	return s == ScopeAuto || s == ScopeStaged || s == ScopeWorktree
}

//constable:nonmutating
func validFormat(f string) bool {
	return f == FormatText || f == FormatJSON
}

//constable:nonmutating
func observe(
	repo *git.Repository,
	scope string,
) ([]refactor.FileChange, gitchange.Scope, error) {
	switch scope {
	case ScopeStaged:
		changes, err := gitchange.StagedChanges(repo)
		return changes, gitchange.ScopeStaged, err
	case ScopeWorktree:
		changes, err := gitchange.WorktreeChanges(repo)
		return changes, gitchange.ScopeWorktree, err
	default:
		return gitchange.ObserveChanges(repo)
	}
}

//constable:nonmutating
func fileWord(n int) string {
	if n == 1 {
		return "1 Go file"
	}
	return fmt.Sprintf("%d Go files", n)
}

func writeReport(
	w io.Writer,
	format string,
	scope gitchange.Scope,
	files int,
	findings []refactor.Refactoring,
) error {
	if format == FormatJSON {
		return writeJSON(w, scope, files, findings)
	}
	if len(findings) == 0 {
		_, err := fmt.Fprintf(w, "no refactorings found in %s changes (%s checked)\n",
			scope, fileWord(files))
		return err
	}
	if _, err := fmt.Fprintf(w, "scope: %s (%s)\n", scope, fileWord(files)); err != nil {
		return err
	}
	for _, r := range findings {
		if _, err := fmt.Fprintf(w, "%s: %s\n", r.Path(), r.Describe()); err != nil {
			return err
		}
	}
	return nil
}

type findingJSON struct {
	Kind        string `json:"kind"`
	File        string `json:"file"`
	Description string `json:"description"`
}

type reportJSON struct {
	Scope    string        `json:"scope"`
	Files    int           `json:"files"`
	Findings []findingJSON `json:"findings"`
}

func writeJSON(
	w io.Writer,
	scope gitchange.Scope,
	files int,
	findings []refactor.Refactoring,
) error {
	report := reportJSON{Scope: string(scope), Files: files, Findings: []findingJSON{}}
	for _, r := range findings {
		report.Findings = append(report.Findings, findingJSON{
			Kind:        string(r.Kind()),
			File:        r.Path(),
			Description: r.Describe(),
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// NewRootCmd builds the intent command. It only reads the repository.
// version is the binary version reported by --version.
func NewRootCmd(version string) *cobra.Command {
	var repoPath, scopeOpt, formatOpt string
	cmd := &cobra.Command{
		Use:     "intent",
		Version: version,
		Short:   "List Go refactorings in observed changes",
		Long: `Observe recent Go changes and list detected refactorings.

When staged changes are present only the staged set (HEAD versus the
index) is observed; otherwise the worktree set is observed. Override
with --scope.

intent only reads the repository: it never stages, commits, or edits
files, so a failed run leaves the repo untouched.`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := git.PlainOpen(repoPath)
			if err != nil {
				return fmt.Errorf("open repository at %q: %w", repoPath, err)
			}
			return Run(cmd.Context(), repo,
				Options{Scope: scopeOpt, Format: formatOpt},
				cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&repoPath, "repo", ".", "path to the git repository")
	cmd.Flags().StringVar(&scopeOpt, "scope", ScopeAuto, "observed changes: auto|staged|worktree")
	cmd.Flags().StringVar(&formatOpt, "format", FormatText, "output format: text|json")
	return cmd
}

// Execute runs the CLI with fang styling.
// version is the binary version reported by --version.
func Execute(ctx context.Context, args []string, stdout, stderr io.Writer, version string) error {
	root := NewRootCmd(version)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	return charmfang.Execute(ctx, root, charmfang.WithVersion(version))
}
