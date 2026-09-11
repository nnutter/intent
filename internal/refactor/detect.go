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
	beforeFile, _, err := parseGoFile(path, before)
	if err != nil {
		return nil, err
	}
	afterFile, afterFset, err := parseGoFile(path, after)
	if err != nil {
		return nil, err
	}
	beforeFuncs := collectFuncs(beforeFile)
	afterFuncs := collectFuncs(afterFile)

	var out []Refactoring
	out = append(out, detectRenames(path, afterFset, beforeFuncs, afterFuncs)...)
	out = append(out, detectVariableRefactorings(path, afterFset, beforeFuncs, afterFuncs)...)
	out = append(out, detectFunctionRefactorings(path, afterFset, beforeFuncs, afterFuncs)...)
	sortRefactorings(out)
	return out, nil
}

//constable:nonmutating
func parseGoFile(path string, src []byte) (*ast.File, *token.FileSet, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, nil, err
	}
	return f, fset, nil
}

// posLine returns the 1-based physical line of pos in fset, or 0.
//
//constable:nonmutating
func posLine(fset *token.FileSet, pos token.Pos) int {
	if fset == nil || !pos.IsValid() {
		return 0
	}
	return fset.PositionFor(pos, false).Line
}
