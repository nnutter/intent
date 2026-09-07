// Package refactor detects Go refactorings between before and after sources.
//
// The MVP focuses on five local refactorings: variable renames, extract
// variable, inline variable, extract function, and inline function.
// Detection is purely syntactic and works on in-memory source bytes so it
// can be tested without touching disk.
package refactor

// Kind identifies the kind of refactoring that was detected.
type Kind string

// Supported refactoring kinds.
const (
	KindRenameVariable  Kind = "rename-variable"
	KindExtractVariable Kind = "extract-variable"
	KindInlineVariable  Kind = "inline-variable"
	KindExtractFunction Kind = "extract-function"
	KindInlineFunction  Kind = "inline-function"
)

// Refactoring is a single detected refactoring in one file.
//
// It is sealed: only the structs declared in this package implement it.
// Switch over concrete types with go-check-sumtype exhaustiveness.
type Refactoring interface {
	// Kind returns the refactoring kind.
	Kind() Kind
	// Path returns the file path the refactoring was found in.
	Path() string
	// Describe returns a short human-readable description.
	Describe() string

	sealedRefactoring()
}

// FileChange holds the before and after contents of one changed Go file.
// A nil Before means the file was added, a nil After means it was deleted.
type FileChange struct {
	Path   string
	Before []byte
	After  []byte
}

// RenameVariable records a local variable renamed within one function.
type RenameVariable struct {
	File       string
	Function   string
	BeforeName string
	AfterName  string
}

// Describe returns a human-readable description.
func (r RenameVariable) Describe() string {
	return "rename variable " + r.BeforeName + " to " + r.AfterName + " in " + r.Function
}

// Kind returns KindRenameVariable.
func (r RenameVariable) Kind() Kind { return KindRenameVariable }

// Path returns the file path.
func (r RenameVariable) Path() string { return r.File }

func (RenameVariable) sealedRefactoring() {}

// ExtractVariable records an expression extracted into a new variable.
type ExtractVariable struct {
	File     string
	Function string
	VarName  string
}

// Describe returns a human-readable description.
func (r ExtractVariable) Describe() string {
	return "extract variable " + r.VarName + " in " + r.Function
}

// Kind returns KindExtractVariable.
func (r ExtractVariable) Kind() Kind { return KindExtractVariable }

// Path returns the file path.
func (r ExtractVariable) Path() string { return r.File }

func (ExtractVariable) sealedRefactoring() {}

// InlineVariable records a variable inlined back into its use sites.
type InlineVariable struct {
	File     string
	Function string
	VarName  string
}

// Describe returns a human-readable description.
func (r InlineVariable) Describe() string {
	return "inline variable " + r.VarName + " in " + r.Function
}

// Kind returns KindInlineVariable.
func (r InlineVariable) Kind() Kind { return KindInlineVariable }

// Path returns the file path.
func (r InlineVariable) Path() string { return r.File }

func (InlineVariable) sealedRefactoring() {}

// ExtractFunction records statements extracted into a new function.
type ExtractFunction struct {
	File           string
	SourceFunction string
	NewFunction    string
}

// Describe returns a human-readable description.
func (r ExtractFunction) Describe() string {
	return "extract function " + r.NewFunction + " from " + r.SourceFunction
}

// Kind returns KindExtractFunction.
func (r ExtractFunction) Kind() Kind { return KindExtractFunction }

// Path returns the file path.
func (r ExtractFunction) Path() string { return r.File }

func (ExtractFunction) sealedRefactoring() {}

// InlineFunction records a function inlined back into its caller.
type InlineFunction struct {
	File            string
	TargetFunction  string
	InlinedFunction string
}

// Describe returns a human-readable description.
func (r InlineFunction) Describe() string {
	return "inline function " + r.InlinedFunction + " into " + r.TargetFunction
}

// Kind returns KindInlineFunction.
func (r InlineFunction) Kind() Kind { return KindInlineFunction }

// Path returns the file path.
func (r InlineFunction) Path() string { return r.File }

func (InlineFunction) sealedRefactoring() {}
