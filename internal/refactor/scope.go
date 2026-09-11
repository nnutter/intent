package refactor

import (
	"go/ast"
	"go/token"
	"slices"
)

// varKind classifies how a variable was defined.
type varKind int

const (
	kindDefine varKind = iota // x := ...
	kindVar                   // var x = ...
	kindParam                 // func params and named results
	kindRange                 // for x := range ... (when :=)
)

// varDef is one variable definition and its use count.
type varDef struct {
	name       string
	kind       varKind
	scopeDepth int
	order      int
	initHash   uint64
	initExpr   ast.Expr
	typeHash   uint64
	useCount   int
	pos        token.Pos
}

// funcName returns the match key for a function: plain name, or
// "(Recv).Name" for methods.
//
//constable:nonmutating
func funcName(fn *ast.FuncDecl) string {
	if fn == nil || fn.Name == nil {
		return ""
	}
	if fn.Recv == nil {
		return fn.Name.Name
	}
	recv := ""
	if len(fn.Recv.List) > 0 {
		recv = recvTypeName(fn.Recv.List[0].Type)
	}
	if recv == "" {
		return fn.Name.Name
	}
	return "(" + recv + ")." + fn.Name.Name
}

// recvTypeName strips pointers and type arguments to a base type name.
//
//constable:nonmutating
func recvTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return recvTypeName(t.X)
	case *ast.IndexExpr:
		return recvTypeName(t.X)
	case *ast.IndexListExpr:
		return recvTypeName(t.X)
	default:
		return ""
	}
}

// simpleFuncName strips a receiver qualifier: "(T).Foo" -> "Foo".
//
//constable:nonmutating
func simpleFuncName(qualified string) string {
	for i := len(qualified) - 1; i >= 0; i-- {
		if qualified[i] == '.' {
			return qualified[i+1:]
		}
	}
	return qualified
}

// collectFuncs indexes top-level functions and methods by qualified name.
//
//constable:nonmutating
func collectFuncs(f *ast.File) map[string]*ast.FuncDecl {
	out := make(map[string]*ast.FuncDecl)
	if f == nil {
		return out
	}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		out[funcName(fn)] = fn
	}
	return out
}

// varCollector walks one function body tracking lexical scopes.
type varCollector struct {
	scopes []*map[string]*varDef
	all    []*varDef
	order  int
}

//constable:nonmutating
func newVarCollector() *varCollector {
	m := make(map[string]*varDef)
	return &varCollector{scopes: []*map[string]*varDef{&m}}
}

func (c *varCollector) current() map[string]*varDef { return *c.scopes[len(c.scopes)-1] }

func (c *varCollector) define(name string, kind varKind, init ast.Expr, typ ast.Expr, pos token.Pos) {
	if name == "" || name == "_" {
		return
	}
	var initHash uint64
	if init != nil {
		initHash = hashExprOf(init, nil)
	}
	var typeHash uint64
	if typ != nil {
		typeHash = hashExprOf(typ, nil)
	}
	d := &varDef{
		name:       name,
		kind:       kind,
		scopeDepth: c.depth(),
		order:      c.order,
		initHash:   initHash,
		initExpr:   init,
		typeHash:   typeHash,
		pos:        pos,
	}
	c.order++
	c.all = append(c.all, d)
	c.current()[name] = d
}

func (c *varCollector) depth() int { return len(c.scopes) - 1 }

func (c *varCollector) pop() {
	if len(c.scopes) > 1 {
		c.scopes = c.scopes[:len(c.scopes)-1]
	}
}

func (c *varCollector) push() {
	m := make(map[string]*varDef)
	c.scopes = append(c.scopes, &m)
}

func (c *varCollector) use(name string) {
	if name == "" || name == "_" {
		return
	}
	for _, v := range slices.Backward(c.scopes) {
		if d, ok := (*v)[name]; ok {
			d.useCount++
			return
		}
	}
}

func (c *varCollector) walkAssign(t *ast.AssignStmt) {
	if t.Tok == token.DEFINE {
		// Define new names in the current scope; existing names in the
		// same scope are assignments (uses).
		cur := c.current()
		for _, lhs := range t.Lhs {
			id, ok := lhs.(*ast.Ident)
			if !ok {
				c.walkExpr(lhs)
				continue
			}
			if id.Name == "_" {
				continue
			}
			if _, exists := cur[id.Name]; exists {
				c.use(id.Name)
				continue
			}
			var init ast.Expr
			if len(t.Rhs) == 1 {
				init = t.Rhs[0]
			} else if len(t.Rhs) > 1 {
				// Multiple RHS: use the whole tuple shape via first match.
				// Store nil and rely on type/use matching; init stays nil
				// to avoid false pairing on tuple unpacking.
				init = nil
			}
			c.define(id.Name, kindDefine, init, nil, id.Pos())
		}
		for _, rhs := range t.Rhs {
			c.walkExpr(rhs)
		}
		return
	}
	for _, lhs := range t.Lhs {
		c.walkExpr(lhs)
	}
	for _, rhs := range t.Rhs {
		c.walkExpr(rhs)
	}
}

func (c *varCollector) walkDecl(d ast.Decl) {
	gd, ok := d.(*ast.GenDecl)
	if !ok {
		return
	}
	for _, s := range gd.Specs {
		vs, ok := s.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, v := range vs.Values {
			c.walkExpr(v)
		}
		if vs.Type != nil {
			c.walkExpr(vs.Type)
		}
		for _, n := range vs.Names {
			c.define(n.Name, kindVar, singleValue(vs.Values), vs.Type, n.Pos())
		}
	}
}

func (c *varCollector) walkExpr(e ast.Expr) {
	if e == nil {
		return
	}
	switch t := e.(type) {
	case *ast.Ident:
		c.use(t.Name)
	case *ast.Ellipsis:
		c.walkExpr(t.Elt)
	case *ast.BasicLit:
		// No variables.
	case *ast.FuncLit:
		c.push()
		c.walkFieldList(t.Type.Params, kindParam)
		if t.Type != nil {
			c.walkFieldList(t.Type.Results, kindParam)
		}
		c.walkStmt(t.Body)
		c.pop()
	case *ast.CompositeLit:
		c.walkExpr(t.Type)
		for _, elt := range t.Elts {
			c.walkExpr(elt)
		}
	case *ast.ParenExpr:
		c.walkExpr(t.X)
	case *ast.SelectorExpr:
		c.walkExpr(t.X)
	case *ast.IndexExpr:
		c.walkExpr(t.X)
		c.walkExpr(t.Index)
	case *ast.IndexListExpr:
		c.walkExpr(t.X)
		for _, ix := range t.Indices {
			c.walkExpr(ix)
		}
	case *ast.SliceExpr:
		c.walkExpr(t.X)
		c.walkExpr(t.Low)
		c.walkExpr(t.High)
		c.walkExpr(t.Max)
	case *ast.TypeAssertExpr:
		c.walkExpr(t.X)
		c.walkExpr(t.Type)
	case *ast.CallExpr:
		c.walkExpr(t.Fun)
		for _, a := range t.Args {
			c.walkExpr(a)
		}
	case *ast.StarExpr:
		c.walkExpr(t.X)
	case *ast.UnaryExpr:
		c.walkExpr(t.X)
	case *ast.BinaryExpr:
		c.walkExpr(t.X)
		c.walkExpr(t.Y)
	case *ast.KeyValueExpr:
		c.walkExpr(t.Key)
		c.walkExpr(t.Value)
	case *ast.ArrayType:
		c.walkExpr(t.Len)
		c.walkExpr(t.Elt)
	case *ast.StructType:
		c.walkFieldList(t.Fields, kindParam)
	case *ast.FuncType:
		c.walkFieldList(t.TypeParams, kindParam)
		c.walkFieldList(t.Params, kindParam)
		c.walkFieldList(t.Results, kindParam)
	case *ast.InterfaceType:
		c.walkFieldList(t.Methods, kindParam)
	case *ast.MapType:
		c.walkExpr(t.Key)
		c.walkExpr(t.Value)
	case *ast.ChanType:
		c.walkExpr(t.Value)
	default:
		ast.Inspect(e, func(n ast.Node) bool {
			if n == e {
				return true
			}
			if id, ok := n.(*ast.Ident); ok {
				c.use(id.Name)
			}
			return true
		})
	}
}

func (c *varCollector) walkFieldList(fl *ast.FieldList, kind varKind) {
	if fl == nil {
		return
	}
	for _, f := range fl.List {
		if f == nil {
			continue
		}
		// Walk the type first so uses inside are counted before defs.
		if f.Type != nil {
			c.walkExpr(f.Type)
		}
		for _, n := range f.Names {
			if n == nil {
				continue
			}
			c.define(n.Name, kind, nil, f.Type, n.Pos())
		}
	}
}

func (c *varCollector) walkRange(t *ast.RangeStmt) {
	c.walkExpr(t.X)
	if t.Tok == token.DEFINE {
		for _, e := range []ast.Expr{t.Key, t.Value} {
			id, ok := e.(*ast.Ident)
			if !ok || id == nil {
				if e != nil {
					c.walkExpr(e)
				}
				continue
			}
			if id.Name == "_" {
				continue
			}
			if _, exists := c.current()[id.Name]; exists {
				c.use(id.Name)
				continue
			}
			c.define(id.Name, kindRange, nil, nil, id.Pos())
		}
	} else {
		if t.Key != nil {
			c.walkExpr(t.Key)
		}
		if t.Value != nil {
			c.walkExpr(t.Value)
		}
	}
	c.walkStmt(t.Body)
}

func (c *varCollector) walkStmt(s ast.Stmt) {
	if s == nil {
		return
	}
	switch t := s.(type) {
	case *ast.BlockStmt:
		c.push()
		for _, st := range t.List {
			c.walkStmt(st)
		}
		c.pop()
	case *ast.DeclStmt:
		c.walkDecl(t.Decl)
	case *ast.ExprStmt:
		c.walkExpr(t.X)
	case *ast.SendStmt:
		c.walkExpr(t.Chan)
		c.walkExpr(t.Value)
	case *ast.IncDecStmt:
		c.walkExpr(t.X)
	case *ast.AssignStmt:
		c.walkAssign(t)
	case *ast.GoStmt:
		c.walkExpr(t.Call)
	case *ast.DeferStmt:
		c.walkExpr(t.Call)
	case *ast.ReturnStmt:
		for _, e := range t.Results {
			c.walkExpr(e)
		}
	case *ast.BranchStmt:
		// Labels are not variables; ignore.
	case *ast.LabeledStmt:
		c.walkStmt(t.Stmt)
	case *ast.IfStmt:
		if t.Init != nil {
			c.walkStmt(t.Init)
		}
		c.walkExpr(t.Cond)
		c.walkStmt(t.Body)
		if t.Else != nil {
			c.walkStmt(t.Else)
		}
	case *ast.SwitchStmt:
		if t.Init != nil {
			c.walkStmt(t.Init)
		}
		c.walkExpr(t.Tag)
		c.walkStmt(t.Body)
	case *ast.TypeSwitchStmt:
		if t.Init != nil {
			c.walkStmt(t.Init)
		}
		c.walkStmt(t.Assign)
		c.walkStmt(t.Body)
	case *ast.SelectStmt:
		c.walkStmt(t.Body)
	case *ast.ForStmt:
		if t.Init != nil {
			c.walkStmt(t.Init)
		}
		c.walkExpr(t.Cond)
		if t.Post != nil {
			c.walkStmt(t.Post)
		}
		c.walkStmt(t.Body)
	case *ast.RangeStmt:
		c.walkRange(t)
	case *ast.CaseClause:
		for _, e := range t.List {
			c.walkExpr(e)
		}
		for _, st := range t.Body {
			c.walkStmt(st)
		}
	case *ast.CommClause:
		if t.Comm != nil {
			c.walkStmt(t.Comm)
		}
		for _, st := range t.Body {
			c.walkStmt(st)
		}
	default:
		// Fall back to inspecting children for uses (e.g. future nodes).
		ast.Inspect(s, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				c.use(id.Name)
			}
			return true
		})
	}
}

// singleValue returns the sole value or nil for multi-value specs.
//
//constable:nonmutating
func singleValue(vals []ast.Expr) ast.Expr {
	if len(vals) == 1 {
		return vals[0]
	}
	return nil
}

// collectVarGroups returns variable definitions grouped by name.
//
//constable:nonmutating
func collectVarGroups(fn *ast.FuncDecl) map[string][]*varDef {
	c := newVarCollector()
	if fn == nil || fn.Type == nil {
		return map[string][]*varDef{}
	}
	c.walkFieldList(fn.Type.Params, kindParam)
	c.walkFieldList(fn.Type.Results, kindParam)
	if fn.Body != nil {
		// Function body block manages its own scope; walk statements
		// directly so params stay at depth 0.
		for _, st := range fn.Body.List {
			c.walkStmt(st)
		}
	}
	groups := make(map[string][]*varDef, len(c.all))
	for _, d := range c.all {
		groups[d.name] = append(groups[d.name], d)
	}
	return groups
}

// singletons keeps only names defined exactly once (no shadowing).
//
//constable:nonmutating
func singletons(groups map[string][]*varDef) map[string]*varDef {
	out := make(map[string]*varDef, len(groups))
	for name, defs := range groups {
		if len(defs) == 1 {
			out[name] = defs[0]
		}
	}
	return out
}
