package refactor

import (
	"go/ast"
	"go/token"
)

// containsCall reports whether a function body calls simpleName, either as
// f() or x.f().
//
//constable:nonmutating
func containsCall(fn *ast.FuncDecl, simpleName string) bool {
	if fn == nil || fn.Body == nil || simpleName == "" {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			if fun.Name == simpleName {
				found = true
				return false
			}
		case *ast.SelectorExpr:
			if fun.Sel != nil && fun.Sel.Name == simpleName {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// containsBlock reports whether body holds a contiguous statement run
// hashing to targetHash.
//
//constable:nonmutating
func containsBlock(body []ast.Stmt, targetHash uint64, targetLen int) bool {
	return findStmtRun(body, targetHash, targetLen) != nil
}

// findStmtRun returns the first contiguous statement run hashing to
// targetHash, or nil if none match.
//
//constable:nonmutating
func findStmtRun(body []ast.Stmt, targetHash uint64, targetLen int) []ast.Stmt {
	if targetLen < 1 || len(body) < targetLen {
		return nil
	}
	for i := 0; i+targetLen <= len(body); i++ {
		run := body[i : i+targetLen]
		if hashStmtListOf(run, nil) == targetHash {
			return run
		}
	}
	return nil
}

// detectFunctionRefactorings finds extract-function and inline-function
// between new or deleted functions and common callers in the same file.
//
//constable:nonmutating
func detectFunctionRefactorings(
	path string,
	afterFset *token.FileSet,
	beforeFuncs, afterFuncs map[string]*ast.FuncDecl,
) []Refactoring {
	var out []Refactoring
	out = append(out, detectExtractFunction(path, afterFset, beforeFuncs, afterFuncs)...)
	out = append(out, detectInlineFunction(path, afterFset, beforeFuncs, afterFuncs)...)
	return out
}

//constable:nonmutating
func detectExtractFunction(
	path string,
	afterFset *token.FileSet,
	beforeFuncs, afterFuncs map[string]*ast.FuncDecl,
) []Refactoring {
	var newNames []string
	for name := range afterFuncs {
		if _, ok := beforeFuncs[name]; !ok {
			newNames = append(newNames, name)
		}
	}
	var common []string
	for name := range beforeFuncs {
		if _, ok := afterFuncs[name]; ok {
			common = append(common, name)
		}
	}
	var out []Refactoring
	for _, newQualified := range newNames {
		newFn := afterFuncs[newQualified]
		if newFn == nil || newFn.Name == nil || newFn.Body == nil || len(newFn.Body.List) == 0 {
			continue
		}
		newBody := newFn.Body.List
		newHash := hashStmtListOf(newBody, nil)
		newSimple := simpleFuncName(newQualified)
		for _, src := range common {
			beforeSrc := beforeFuncs[src]
			afterSrc := afterFuncs[src]
			if beforeSrc == nil || afterSrc == nil ||
				beforeSrc.Body == nil || afterSrc.Body == nil {
				continue
			}
			if !containsCall(afterSrc, newSimple) {
				continue
			}
			if containsCall(beforeSrc, newSimple) {
				continue
			}
			if !containsBlock(beforeSrc.Body.List, newHash, len(newBody)) {
				continue
			}
			if containsBlock(afterSrc.Body.List, newHash, len(newBody)) {
				continue
			}
			out = append(out, ExtractFunction{
				File:           path,
				SourceFunction: src,
				NewFunction:    newQualified,
				AfterLine:      posLine(afterFset, newFn.Name.Pos()),
			})
		}
	}
	return out
}

//constable:nonmutating
func detectInlineFunction(
	path string,
	afterFset *token.FileSet,
	beforeFuncs, afterFuncs map[string]*ast.FuncDecl,
) []Refactoring {
	var deleted []string
	for name := range beforeFuncs {
		if _, ok := afterFuncs[name]; !ok {
			deleted = append(deleted, name)
		}
	}
	var common []string
	for name := range beforeFuncs {
		if _, ok := afterFuncs[name]; ok {
			common = append(common, name)
		}
	}
	var out []Refactoring
	for _, delQualified := range deleted {
		delFn := beforeFuncs[delQualified]
		if delFn == nil || delFn.Body == nil || len(delFn.Body.List) == 0 {
			continue
		}
		delBody := delFn.Body.List
		delHash := hashStmtListOf(delBody, nil)
		delSimple := simpleFuncName(delQualified)
		for _, target := range common {
			beforeT := beforeFuncs[target]
			afterT := afterFuncs[target]
			if beforeT == nil || afterT == nil ||
				beforeT.Body == nil || afterT.Body == nil {
				continue
			}
			if !containsCall(beforeT, delSimple) {
				continue
			}
			if containsCall(afterT, delSimple) {
				continue
			}
			run := findStmtRun(afterT.Body.List, delHash, len(delBody))
			if run == nil {
				continue
			}
			if containsBlock(beforeT.Body.List, delHash, len(delBody)) {
				continue
			}
			line := 0
			if len(run) > 0 && run[0] != nil {
				line = posLine(afterFset, run[0].Pos())
			}
			out = append(out, InlineFunction{
				File:            path,
				TargetFunction:  target,
				InlinedFunction: delQualified,
				AfterLine:       line,
			})
		}
	}
	return out
}
