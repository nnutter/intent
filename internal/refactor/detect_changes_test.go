package refactor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectChangesMultiFile(t *testing.T) {
	t.Parallel()
	changes := []FileChange{
		{
			Path:   "a.go",
			Before: []byte("package p\nfunc f() { x := 1; _ = x }\n"),
			After:  []byte("package p\nfunc f() { y := 1; _ = y }\n"),
		},
		{
			Path: "b.go",
			Before: []byte(`package p
import "fmt"
func g(a, b int) { fmt.Println(a + b) }
`),
			After: []byte(`package p
import "fmt"
func g(a, b int) { s := a + b; fmt.Println(s) }
`),
		},
		{Path: "README.md", Before: []byte("a"), After: []byte("b")},
	}
	got, err := DetectChanges(changes)
	require.NoError(t, err)
	require.Len(t, got, 2)
	kinds := map[Kind]int{}
	for _, r := range got {
		kinds[r.Kind()]++
	}
	require.Equal(t, 1, kinds[KindRenameVariable])
	require.Equal(t, 1, kinds[KindExtractVariable])
}

func TestDetectChangesMixedInOneFile(t *testing.T) {
	t.Parallel()
	before := []byte(`package p
import "fmt"
func f(a, b int) {
	foo := 1
	fmt.Println(foo)
	fmt.Println(a + b)
}
`)
	after := []byte(`package p
import "fmt"
func f(a, b int) {
	bar := 1
	fmt.Println(bar)
	sum := a + b
	fmt.Println(sum)
}
`)
	got, err := DetectChanges([]FileChange{{Path: "a.go", Before: before, After: after}})
	require.NoError(t, err)
	var sawRename, sawExtract bool
	for _, r := range got {
		switch v := r.(type) {
		case RenameVariable:
			if v.BeforeName == "foo" && v.AfterName == "bar" {
				sawRename = true
			}
		case ExtractVariable:
			if v.VarName == "sum" {
				sawExtract = true
			}
		}
	}
	require.True(t, sawRename, "missing rename in %v", got)
	require.True(t, sawExtract, "missing extract in %v", got)
}

func TestDetectChangesParseErrorDoesNotBlockOthers(t *testing.T) {
	t.Parallel()
	changes := []FileChange{
		{Path: "bad.go", Before: []byte("package p\nfunc f() {"), After: []byte("package p\nfunc f() {}")},
		{
			Path:   "good.go",
			Before: []byte("package p\nfunc f() { x := 1; _ = x }\n"),
			After:  []byte("package p\nfunc f() { y := 1; _ = y }\n"),
		},
	}
	got, err := DetectChanges(changes)
	require.Error(t, err)
	require.Len(t, got, 1)
	_, ok := got[0].(RenameVariable)
	require.True(t, ok)
}
