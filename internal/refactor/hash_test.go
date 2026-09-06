package refactor

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func parseExpr(t *testing.T, src string) ast.Expr {
	t.Helper()
	e, err := parser.ParseExpr(src)
	require.NoError(t, err)
	return e
}

func parseStmt(t *testing.T, src string) ast.Stmt {
	t.Helper()
	f, err := parser.ParseFile(
		token.NewFileSet(),
		"test.go",
		"package p\nfunc f() {\n"+src+"\n}",
		0,
	)
	require.NoError(t, err)
	decl, ok := f.Decls[0].(*ast.FuncDecl)
	require.True(t, ok)
	require.NotEmpty(t, decl.Body.List)
	return decl.Body.List[0]
}

func TestHashExprPositionIndependent(t *testing.T) {
	t.Parallel()
	a := parseExpr(t, "a+b")
	b := parseExpr(t, "a+b")
	require.Equal(t, hashExprOf(a, nil), hashExprOf(b, nil))
}

func TestHashExprDistinguishesOperators(t *testing.T) {
	t.Parallel()
	a := parseExpr(t, "a+b")
	b := parseExpr(t, "a-b")
	require.NotEqual(t, hashExprOf(a, nil), hashExprOf(b, nil))
}

func TestHashExprNormalizer(t *testing.T) {
	t.Parallel()
	a := parseExpr(t, "foo+1")
	b := parseExpr(t, "bar+1")
	require.NotEqual(t, hashExprOf(a, nil), hashExprOf(b, nil))
	normA := identNormalizer(func(s string) string {
		if s == "foo" {
			return "#v"
		}
		return s
	})
	normB := identNormalizer(func(s string) string {
		if s == "bar" {
			return "#v"
		}
		return s
	})
	require.Equal(t, hashExprOf(a, normA), hashExprOf(b, normB))
}

func TestHashStmtListOrderSensitive(t *testing.T) {
	t.Parallel()
	a1 := parseStmt(t, "x := 1")
	a2 := parseStmt(t, "y := 2")
	b1 := parseStmt(t, "x := 1")
	b2 := parseStmt(t, "y := 2")
	require.Equal(
		t,
		hashStmtListOf([]ast.Stmt{a1, a2}, nil),
		hashStmtListOf([]ast.Stmt{b1, b2}, nil),
	)
	require.NotEqual(
		t,
		hashStmtListOf([]ast.Stmt{a1, a2}, nil),
		hashStmtListOf([]ast.Stmt{a2, a1}, nil),
	)
}

func TestIsTrivialExpr(t *testing.T) {
	t.Parallel()
	require.True(t, isTrivialExpr(parseExpr(t, "x")))
	require.True(t, isTrivialExpr(parseExpr(t, "42")))
	require.True(t, isTrivialExpr(parseExpr(t, "(x)")))
	require.False(t, isTrivialExpr(parseExpr(t, "a+b")))
	require.False(t, isTrivialExpr(parseExpr(t, "f(x)")))
}
