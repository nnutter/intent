package refactor

import (
	"go/ast"
	"hash"
	"hash/fnv"
	"strconv"
)

// identNormalizer maps an identifier spelling to its canonical form.
// A nil normalizer means identity. Detectors use a normalizer to treat a
// renamed variable as equal on both sides.
type identNormalizer func(string) string

type hasher struct {
	h    hash.Hash64
	norm identNormalizer
}

//constable:nonmutating
func newHasher(norm identNormalizer) *hasher {
	return &hasher{h: fnv.New64a(), norm: norm}
}

func (h *hasher) hashDecl(d ast.Decl) {
	if d == nil {
		h.write("nil-decl")
		return
	}
	switch t := d.(type) {
	case *ast.GenDecl:
		h.write("GenDecl")
		h.write(t.Tok.String())
		h.write(strconv.Itoa(len(t.Specs)))
		for _, s := range t.Specs {
			h.hashSpec(s)
		}
	case *ast.FuncDecl:
		h.write("FuncDecl")
		if t.Name != nil {
			h.write(t.Name.Name)
		}
		if t.Type != nil {
			h.hashFuncType(t.Type)
		}
		h.hashStmt(t.Body)
	default:
		h.write("unknown-decl")
	}
}

func (h *hasher) hashExpr(e ast.Expr) {
	if e == nil {
		h.write("nil-expr")
		return
	}
	switch t := e.(type) {
	case *ast.Ident:
		h.write("Ident")
		h.name(t.Name)
	case *ast.Ellipsis:
		h.write("Ellipsis")
		h.hashExpr(t.Elt)
	case *ast.BasicLit:
		h.write("BasicLit")
		h.write(t.Kind.String())
		h.write(t.Value)
	case *ast.FuncLit:
		h.write("FuncLit")
		h.hashFuncType(t.Type)
		h.hashStmt(t.Body)
	case *ast.CompositeLit:
		h.write("CompositeLit")
		h.hashExpr(t.Type)
		h.write(strconv.Itoa(len(t.Elts)))
		for _, elt := range t.Elts {
			h.hashExpr(elt)
		}
	case *ast.ParenExpr:
		h.write("Paren")
		h.hashExpr(t.X)
	case *ast.SelectorExpr:
		h.write("Selector")
		h.hashExpr(t.X)
		h.write(t.Sel.Name)
	case *ast.IndexExpr:
		h.write("Index")
		h.hashExpr(t.X)
		h.hashExpr(t.Index)
	case *ast.IndexListExpr:
		h.write("IndexList")
		h.hashExpr(t.X)
		h.write(strconv.Itoa(len(t.Indices)))
		for _, ix := range t.Indices {
			h.hashExpr(ix)
		}
	case *ast.SliceExpr:
		h.write("Slice")
		h.hashExpr(t.X)
		h.hashExpr(t.Low)
		h.hashExpr(t.High)
		h.hashExpr(t.Max)
		if t.Slice3 {
			h.write("slice3")
		}
	case *ast.TypeAssertExpr:
		h.write("TypeAssert")
		h.hashExpr(t.X)
		h.hashExpr(t.Type)
	case *ast.CallExpr:
		h.write("Call")
		h.hashExpr(t.Fun)
		h.write(strconv.Itoa(len(t.Args)))
		for _, a := range t.Args {
			h.hashExpr(a)
		}
		if t.Ellipsis.IsValid() {
			h.write("ellipsis")
		}
	case *ast.StarExpr:
		h.write("Star")
		h.hashExpr(t.X)
	case *ast.UnaryExpr:
		h.write("Unary")
		h.write(t.Op.String())
		h.hashExpr(t.X)
	case *ast.BinaryExpr:
		h.write("Binary")
		h.write(t.Op.String())
		h.hashExpr(t.X)
		h.hashExpr(t.Y)
	case *ast.KeyValueExpr:
		h.write("KeyValue")
		h.hashExpr(t.Key)
		h.hashExpr(t.Value)
	case *ast.ArrayType:
		h.write("Array")
		h.hashExpr(t.Len)
		h.hashExpr(t.Elt)
	case *ast.StructType:
		h.write("Struct")
		h.hashFieldList(t.Fields)
	case *ast.FuncType:
		h.write("FuncType")
		h.hashFuncType(t)
	case *ast.InterfaceType:
		h.write("Interface")
		h.hashFieldList(t.Methods)
	case *ast.MapType:
		h.write("Map")
		h.hashExpr(t.Key)
		h.hashExpr(t.Value)
	case *ast.ChanType:
		h.write("Chan")
		h.write(strconv.Itoa(int(t.Dir)))
		h.hashExpr(t.Value)
	default:
		h.write("unknown-expr")
	}
}

func (h *hasher) hashField(f *ast.Field) {
	if f == nil {
		h.write("nil-field")
		return
	}
	h.write("field-begin")
	h.write(strconv.Itoa(len(f.Names)))
	for _, n := range f.Names {
		if n == nil {
			h.write("nil-name")
			continue
		}
		h.name(n.Name)
	}
	h.hashExpr(f.Type)
	if f.Tag != nil {
		h.write(f.Tag.Value)
	}
	h.write("field-end")
}

func (h *hasher) hashFieldList(fl *ast.FieldList) {
	if fl == nil {
		h.write("nil-fieldlist")
		return
	}
	h.write("fieldlist")
	h.write(strconv.Itoa(len(fl.List)))
	for _, f := range fl.List {
		h.hashField(f)
	}
}

func (h *hasher) hashFuncType(t *ast.FuncType) {
	if t == nil {
		h.write("nil-functype")
		return
	}
	h.write("functype-begin")
	h.hashFieldList(t.TypeParams)
	h.hashFieldList(t.Params)
	h.hashFieldList(t.Results)
	h.write("functype-end")
}

func (h *hasher) hashSpec(s ast.Spec) {
	switch t := s.(type) {
	case *ast.ValueSpec:
		h.write("ValueSpec")
		h.write(strconv.Itoa(len(t.Names)))
		for _, n := range t.Names {
			h.name(n.Name)
		}
		h.hashExpr(t.Type)
		h.write(strconv.Itoa(len(t.Values)))
		for _, v := range t.Values {
			h.hashExpr(v)
		}
	case *ast.TypeSpec:
		h.write("TypeSpec")
		h.name(t.Name.Name)
		h.hashFieldList(t.TypeParams)
		h.hashExpr(t.Type)
	case *ast.ImportSpec:
		h.write("ImportSpec")
		if t.Name != nil {
			h.name(t.Name.Name)
		}
		h.hashExpr(t.Path)
	default:
		h.write("unknown-spec")
	}
}

func (h *hasher) hashStmt(s ast.Stmt) {
	if s == nil {
		h.write("nil-stmt")
		return
	}
	switch t := s.(type) {
	case *ast.DeclStmt:
		h.write("DeclStmt")
		h.hashDecl(t.Decl)
	case *ast.LabeledStmt:
		h.write("Labeled")
		if t.Label != nil {
			h.name(t.Label.Name)
		}
		h.hashStmt(t.Stmt)
	case *ast.ExprStmt:
		h.write("ExprStmt")
		h.hashExpr(t.X)
	case *ast.SendStmt:
		h.write("Send")
		h.hashExpr(t.Chan)
		h.hashExpr(t.Value)
	case *ast.IncDecStmt:
		h.write("IncDec")
		h.write(t.Tok.String())
		h.hashExpr(t.X)
	case *ast.AssignStmt:
		h.write("Assign")
		h.write(t.Tok.String())
		h.write(strconv.Itoa(len(t.Lhs)))
		for _, lhs := range t.Lhs {
			h.hashExpr(lhs)
		}
		h.write(strconv.Itoa(len(t.Rhs)))
		for _, rhs := range t.Rhs {
			h.hashExpr(rhs)
		}
	case *ast.GoStmt:
		h.write("Go")
		h.hashExpr(t.Call)
	case *ast.DeferStmt:
		h.write("Defer")
		h.hashExpr(t.Call)
	case *ast.ReturnStmt:
		h.write("Return")
		h.write(strconv.Itoa(len(t.Results)))
		for _, r := range t.Results {
			h.hashExpr(r)
		}
	case *ast.BranchStmt:
		h.write("Branch")
		h.write(t.Tok.String())
		if t.Label != nil {
			h.name(t.Label.Name)
		}
	case *ast.BlockStmt:
		h.write("Block")
		h.hashStmtList(t.List)
	case *ast.IfStmt:
		h.write("If")
		h.hashStmt(t.Init)
		h.hashExpr(t.Cond)
		h.hashStmt(t.Body)
		h.hashStmt(t.Else)
	case *ast.CaseClause:
		h.write("Case")
		h.write(strconv.Itoa(len(t.List)))
		for _, e := range t.List {
			h.hashExpr(e)
		}
		h.hashStmtList(t.Body)
	case *ast.SwitchStmt:
		h.write("Switch")
		h.hashStmt(t.Init)
		h.hashExpr(t.Tag)
		h.hashStmt(t.Body)
	case *ast.TypeSwitchStmt:
		h.write("TypeSwitch")
		h.hashStmt(t.Init)
		h.hashStmt(t.Assign)
		h.hashStmt(t.Body)
	case *ast.CommClause:
		h.write("Comm")
		h.hashStmt(t.Comm)
		h.hashStmtList(t.Body)
	case *ast.SelectStmt:
		h.write("Select")
		h.hashStmt(t.Body)
	case *ast.ForStmt:
		h.write("For")
		h.hashStmt(t.Init)
		h.hashExpr(t.Cond)
		h.hashStmt(t.Post)
		h.hashStmt(t.Body)
	case *ast.RangeStmt:
		h.write("Range")
		h.write(t.Tok.String())
		h.hashExpr(t.Key)
		h.hashExpr(t.Value)
		h.hashExpr(t.X)
		h.hashStmt(t.Body)
	default:
		h.write("unknown-stmt")
	}
}

func (h *hasher) hashStmtList(list []ast.Stmt) {
	h.write("stmtlist")
	h.write(strconv.Itoa(len(list)))
	for _, s := range list {
		h.hashStmt(s)
	}
}

func (h *hasher) name(s string) {
	if h.norm != nil {
		s = h.norm(s)
	}
	h.write(s)
}

func (h *hasher) write(s string) {
	_, _ = h.h.Write([]byte(s))
	_, _ = h.h.Write([]byte{0})
}

// hashExprOf returns a position-independent hash of an expression.
//
//constable:nonmutating
func hashExprOf(e ast.Expr, norm identNormalizer) uint64 {
	h := newHasher(norm)
	h.hashExpr(e)
	return h.h.Sum64()
}

// hashStmtListOf returns a position-independent hash of a statement list.
//
//constable:nonmutating
func hashStmtListOf(list []ast.Stmt, norm identNormalizer) uint64 {
	h := newHasher(norm)
	h.hashStmtList(list)
	return h.h.Sum64()
}

// isTrivialExpr reports whether an expression is too small to be a
// meaningful extract-variable candidate (a bare identifier or literal).
//
//constable:nonmutating
func isTrivialExpr(e ast.Expr) bool {
	switch e := e.(type) {
	case nil:
		return true
	case *ast.Ident, *ast.BasicLit:
		return true
	case *ast.ParenExpr:
		return isTrivialExpr(e.X)
	default:
		return false
	}
}
