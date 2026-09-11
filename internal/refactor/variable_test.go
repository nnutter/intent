package refactor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectExtractVariable(t *testing.T) {
	t.Parallel()
	before := []byte(`package p
import "fmt"
func f(a, b int) {
	fmt.Println(a + b)
}
`)
	after := []byte(`package p
import "fmt"
func f(a, b int) {
	sum := a + b
	fmt.Println(sum)
}
`)
	got, err := Detect("a.go", before, after)
	require.NoError(t, err)
	found := false
	for _, r := range got {
		if e, ok := r.(ExtractVariable); ok && e.VarName == "sum" && e.Function == "f" {
			require.Equal(t, 4, e.Line())
			found = true
		}
	}
	require.True(t, found, "expected extract variable sum, got %v", got)
}

func TestDetectInlineVariable(t *testing.T) {
	t.Parallel()
	before := []byte(`package p
import "fmt"
func f(a, b int) {
	sum := a + b
	fmt.Println(sum)
}
`)
	after := []byte(`package p
import "fmt"
func f(a, b int) {
	fmt.Println(a + b)
}
`)
	got, err := Detect("a.go", before, after)
	require.NoError(t, err)
	found := false
	for _, r := range got {
		if e, ok := r.(InlineVariable); ok && e.VarName == "sum" {
			require.Equal(t, 4, e.Line())
			found = true
		}
	}
	require.True(t, found, "expected inline variable sum, got %v", got)
}

func TestDetectNoExtractForNewCode(t *testing.T) {
	t.Parallel()
	before := []byte("package p\nfunc f(a int) int { return a }\n")
	after := []byte("package p\nfunc f(a int) int { q := a * 2; return a + q }\n")
	got, err := Detect("a.go", before, after)
	require.NoError(t, err)
	for _, r := range got {
		_, isExtract := r.(ExtractVariable)
		require.False(t, isExtract, "unexpected extract: %v", r)
	}
}

func TestDetectNoExtractForTrivialLiteral(t *testing.T) {
	t.Parallel()
	before := []byte("package p\nfunc f() int { return 42 }\n")
	after := []byte("package p\nfunc f() int { x := 42; return x }\n")
	got, err := Detect("a.go", before, after)
	require.NoError(t, err)
	for _, r := range got {
		_, isExtract := r.(ExtractVariable)
		require.False(t, isExtract, "trivial literal should not report: %v", r)
	}
}

func TestDetectNoExtractForNamedDiscardedResult(t *testing.T) {
	t.Parallel()
	before := []byte(`package p
func g() (int, error) { return 1, nil }
func commit() {
	_, err := g()
	if err != nil {
		return
	}
}
`)
	after := []byte(`package p
func g() (int, error) { return 1, nil }
func commit() int {
	h, err := g()
	if err != nil {
		return 0
	}
	return h
}
`)
	got, err := Detect("a.go", before, after)
	require.NoError(t, err)
	for _, r := range got {
		_, isExtract := r.(ExtractVariable)
		require.False(t, isExtract, "naming a discarded result is not extract: %v", r)
	}
}

func TestDetectNoInlineForSameNameInSwitchCases(t *testing.T) {
	t.Parallel()
	before := []byte(`package p
func staged() ([]int, string, error)  { return nil, "staged", nil }
func worktree() ([]int, string, error) { return nil, "worktree", nil }
func observe(kind string) ([]int, string, error) {
	switch kind {
	case "staged":
		changes, err := staged()
		return changes, "staged", err
	case "worktree":
		changes, err := worktree()
		return changes, "worktree", err
	default:
		return staged()
	}
}
`)
	after := []byte(`package p
func staged() ([]int, string, error)  { return nil, "staged", nil }
func worktree() ([]int, string, error) { return nil, "worktree", nil }
func commit() ([]int, string, error)   { return nil, "commit", nil }
func observe(kind string) ([]int, string, error) {
	if kind == "commit" {
		changes, err := commit()
		return changes, "commit", err
	}
	switch kind {
	case "staged":
		changes, err := staged()
		return changes, "staged", err
	case "worktree":
		changes, err := worktree()
		return changes, "worktree", err
	default:
		changes, scope, err := staged()
		return changes, scope, err
	}
}
`)
	got, err := Detect("a.go", before, after)
	require.NoError(t, err)
	for _, r := range got {
		_, isInline := r.(InlineVariable)
		require.False(t, isInline, "same-name switch locals are not inline: %v", r)
	}
}
