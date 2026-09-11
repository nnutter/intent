package refactor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectRenameVariable(t *testing.T) {
	t.Parallel()
	before := []byte(`package p
import "fmt"
func greet() {
	name := "bob"
	fmt.Println(name)
}
`)
	after := []byte(`package p
import "fmt"
func greet() {
	user := "bob"
	fmt.Println(user)
}
`)
	got, err := Detect("a.go", before, after)
	require.NoError(t, err)
	require.Len(t, got, 1)
	r, ok := got[0].(RenameVariable)
	require.True(t, ok)
	require.Equal(t, "a.go", r.Path())
	require.Equal(t, "greet", r.Function)
	require.Equal(t, "name", r.BeforeName)
	require.Equal(t, "user", r.AfterName)
	require.Equal(t, 4, r.Line())
}

func TestDetectRenameParam(t *testing.T) {
	t.Parallel()
	before := []byte("package p\nfunc f(foo int) int { return foo + 1 }\n")
	after := []byte("package p\nfunc f(bar int) int { return bar + 1 }\n")
	got, err := Detect("a.go", before, after)
	require.NoError(t, err)
	require.Len(t, got, 1)
	r, ok := got[0].(RenameVariable)
	require.True(t, ok)
	require.Equal(t, "foo", r.BeforeName)
	require.Equal(t, "bar", r.AfterName)
	require.Equal(t, 2, r.Line())
}

func TestDetectNoRenameWhenIdentical(t *testing.T) {
	t.Parallel()
	src := []byte("package p\nfunc f() { x := 1; _ = x }\n")
	got, err := Detect("a.go", src, src)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestDetectNoRenameOnTypeChange(t *testing.T) {
	t.Parallel()
	before := []byte("package p\nfunc f() { x := 1; _ = x }\n")
	after := []byte("package p\nfunc f() { y := \"hi\"; _ = y }\n")
	got, err := Detect("a.go", before, after)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestDetectRenameMixedWithOtherEdits(t *testing.T) {
	t.Parallel()
	before := []byte(`package p
import "fmt"
func f() {
	x := 1
	fmt.Println(x)
}
`)
	after := []byte(`package p
import "fmt"
func f() {
	y := 1
	fmt.Println(y)
	fmt.Println("extra")
}
`)
	got, err := Detect("a.go", before, after)
	require.NoError(t, err)
	require.Len(t, got, 1)
	r, ok := got[0].(RenameVariable)
	require.True(t, ok)
	require.Equal(t, "x", r.BeforeName)
	require.Equal(t, "y", r.AfterName)
}
