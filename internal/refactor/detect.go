package refactor

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
)

// Detect compares before and after Go sources from one file and returns
// the refactorings found, sorted for determinism.
//
// Identical inputs return nil without parsing. Added or deleted files
// (one side empty) return nil. Unparseable inputs return an error.
//
//constable:nonmutating
func Detect(path string, before, after []byte) ([]Refactoring, error) {
	if bytes.Equal(before, after) {
		return nil, nil
	}
	if len(before) == 0 || len(after) == 0 {
		return nil, nil
	}
	beforeFile, err := parseGoFile(path, before)
	if err != nil {
		return nil, err
	}
	afterFile, err := parseGoFile(path, after)
	if err != nil {
		return nil, err
	}
	beforeFuncs := collectFuncs(beforeFile)
	afterFuncs := collectFuncs(afterFile)

	var out []Refactoring
	out = append(out, detectRenames(path, beforeFuncs, afterFuncs)...)
	out = append(out, detectVariableRefactorings(path, beforeFuncs, afterFuncs)...)
	out = append(out, detectFunctionRefactorings(path, beforeFuncs, afterFuncs)...)
	sortRefactorings(out)
	return out, nil
}

//constable:nonmutating
func parseGoFile(path string, src []byte) (*ast.File, error) {
	fset := token.NewFileSet()
	return parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
}
