package refactor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectExtractFunction(t *testing.T) {
	t.Parallel()
	before := []byte(`package p
import "fmt"
func foo() {
	a := 1
	b := 2
	fmt.Println(a + b)
}
`)
	after := []byte(`package p
import "fmt"
func foo() {
	bar()
}
func bar() {
	a := 1
	b := 2
	fmt.Println(a + b)
}
`)
	got, err := Detect("a.go", before, after)
	require.NoError(t, err)
	found := false
	for _, r := range got {
		if e, ok := r.(ExtractFunction); ok &&
			e.SourceFunction == "foo" && e.NewFunction == "bar" {
			found = true
		}
	}
	require.True(t, found, "expected extract bar from foo, got %v", got)
}

func TestDetectInlineFunction(t *testing.T) {
	t.Parallel()
	before := []byte(`package p
import "fmt"
func foo() {
	bar()
}
func bar() {
	a := 1
	b := 2
	fmt.Println(a + b)
}
`)
	after := []byte(`package p
import "fmt"
func foo() {
	a := 1
	b := 2
	fmt.Println(a + b)
}
`)
	got, err := Detect("a.go", before, after)
	require.NoError(t, err)
	found := false
	for _, r := range got {
		if e, ok := r.(InlineFunction); ok &&
			e.TargetFunction == "foo" && e.InlinedFunction == "bar" {
			found = true
		}
	}
	require.True(t, found, "expected inline bar into foo, got %v", got)
}

func TestDetectNoExtractForUnrelatedFunc(t *testing.T) {
	t.Parallel()
	before := []byte("package p\nfunc foo() {}\n")
	after := []byte("package p\nfunc foo() {}\nfunc bar() int { return 42 }\n")
	got, err := Detect("a.go", before, after)
	require.NoError(t, err)
	for _, r := range got {
		_, isExtract := r.(ExtractFunction)
		require.False(t, isExtract, "unexpected extract: %v", r)
	}
}

func TestDetectNoExtractWithoutCall(t *testing.T) {
	t.Parallel()
	before := []byte(`package p
func foo() {
	println("hi")
	println("there")
}
`)
	after := []byte(`package p
func foo() {
	println("hi")
	println("there")
}
func bar() {
	println("hi")
	println("there")
}
`)
	got, err := Detect("a.go", before, after)
	require.NoError(t, err)
	for _, r := range got {
		_, isExtract := r.(ExtractFunction)
		require.False(t, isExtract, "duplicate without call should not report: %v", r)
	}
}
