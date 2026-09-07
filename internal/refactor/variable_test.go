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
