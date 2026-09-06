package refactor

import (
	"go/ast"
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
	if targetLen < 1 || len(body) < targetLen {
		return false
	}
	for i := 0; i+targetLen <= len(body); i++ {
		if hashStmtListOf(body[i:i+targetLen], nil) == targetHash {
			return true
		}
	}
	return false
}

// detectFunctionRefactorings finds extract-function and inline-function
// between new or deleted functions and common callers in the same file.
//
//constable:nonmutating
func detectFunctionRefactorings(
	path string,
	beforeFuncs, afterFuncs map[string]*ast.FuncDecl,
) []Refactoring {
	var out []Refactoring
	out = append(out, detectExtractFunction(path, beforeFuncs, afterFuncs)...)
	out = append(out, detectInlineFunction(path, beforeFuncs, afterFuncs)...)
	return out
}

//constable:nonmutating
func detectExtractFunction(
	path string,
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
		if newFn == nil || newFn.Body == nil || len(newFn.Body.List) == 0 {
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
			})
		}
	}
	return out
}

//constable:nonmutating
func detectInlineFunction(
	path string,
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
			if !containsBlock(afterT.Body.List, delHash, len(delBody)) {
				continue
			}
			if containsBlock(beforeT.Body.List, delHash, len(delBody)) {
				continue
			}
			out = append(out, InlineFunction{
				File:            path,
				TargetFunction:  target,
				InlinedFunction: delQualified,
			})
		}
	}
	return out
}
