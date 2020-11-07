package analyzer

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const maxDeclLength = 80 // TODO: flag
const maxDeclHeight = 1

// Analyzer is an analysis.Analyzer instance for ifshort linter.
var Analyzer = &analysis.Analyzer{
	Name:     "ifshort",
	Doc:      "Checks that your code uses short syntax for if-statements whenever possible.",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

type occurrenceInfo struct {
	declarationPos token.Pos
	ifStmtPos      token.Pos
}

func (oi occurrenceInfo) maxPos() token.Pos {
	if oi.declarationPos > oi.ifStmtPos {
		return oi.declarationPos
	}
	return oi.ifStmtPos
}

/*
https://medium.com/justforfunc/understanding-go-programs-with-go-parser-c4e88a6edb87

https://astexplorer.net/

https://disaev.me/p/writing-useful-go-analysis-linter/
*/

func run(pass *analysis.Pass) (interface{}, error) {
	inspector := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
	}

	inspector.Preorder(nodeFilter, func(node ast.Node) {
		fdecl := node.(*ast.FuncDecl)

		/*if fdecl.Name.Name != "" {
			return
		}*/

		candidates := map[string]occurrenceInfo{}

		for varName, occ := range getOccurrenceMap(fdecl, pass) {
			if occ.maxPos() == occ.ifStmtPos && occ.declarationPos != 0 {
				candidates[varName] = occ
			}
		}

		for _, stmt := range fdecl.Body.List {
			switch v := stmt.(type) {
			case *ast.AssignStmt:
				for _, el := range v.Rhs {
					checkCandidate(candidates, el)
				}
			case *ast.DeferStmt:
				for _, a := range v.Call.Args {
					checkCandidate(candidates, a)
				}
			case *ast.ExprStmt:
				if callExpr, ok := v.X.(*ast.CallExpr); ok {
					checkCandidate(candidates, callExpr)
				}
			case *ast.IfStmt:
				switch cond := v.Cond.(type) {
				case *ast.BinaryExpr:
					checkCandidate(candidates, cond.X)
					checkCandidate(candidates, cond.Y)
				case *ast.CallExpr:
					checkCandidate(candidates, cond)
				}
				if init, ok := v.Init.(*ast.AssignStmt); ok {
					for _, e := range init.Rhs {
						checkCandidate(candidates, e)
					}
				}
			case *ast.GoStmt:
				for _, a := range v.Call.Args {
					checkCandidate(candidates, a)
				}
			case *ast.ReturnStmt:
				for _, r := range v.Results {
					checkCandidate(candidates, r)
				}
			case *ast.RangeStmt:
				checkCandidate(candidates, v.X)
			case *ast.SendStmt:
				checkCandidate(candidates, v.Value)
			case *ast.SwitchStmt:
				checkCandidate(candidates, v.Tag)
				for _, el := range v.Body.List {
					if clauses, ok := el.(*ast.CaseClause); ok {
						for _, c := range clauses.List {
							switch v := c.(type) {
							case *ast.BinaryExpr:
								checkCandidate(candidates, v.X)
								checkCandidate(candidates, v.Y)
							case *ast.Ident:
								checkCandidate(candidates, v)
							}
						}
						for _, c := range clauses.Body {
							if est, ok := c.(*ast.ExprStmt); ok {
								checkCandidate(candidates, est.X)
							}
						}
					}
				}
			}
		}

		for varName := range candidates {
			pass.Reportf(candidates[varName].declarationPos,
				"variable '%s' is only used in the if-statement (%s); consider using short syntax",
				varName, pass.Fset.Position(candidates[varName].ifStmtPos))
		}
	})
	return nil, nil
}

func processIdents(vars map[string]occurrenceInfo, idents ...ast.Expr) {
	for _, v := range idents {
		ident, ok := v.(*ast.Ident)
		if !ok {
			continue
		}
		if ui, ok := vars[ident.Name]; ok {
			if ui.ifStmtPos != 0 && ui.declarationPos != 0 {
				continue
			}
			ui.ifStmtPos = v.Pos()
			vars[ident.Name] = ui
		} else if ident.Name != "nil" {
			vars[ident.Name] = occurrenceInfo{ifStmtPos: v.Pos()}
		}
	}
}

func checkCandidate(candidates map[string]occurrenceInfo, e ast.Expr) {
	switch v := e.(type) {
	case *ast.Ident:
		if v.Pos() != candidates[v.Name].maxPos() {
			delete(candidates, v.Name)
		}
	case *ast.CallExpr:
		processCallExpr(v, candidates)
	case *ast.UnaryExpr:
		checkCandidate(candidates, v.X)
	}
}

func processCallExpr(e ast.Expr, candidates map[string]occurrenceInfo) {
	if callExpr, ok := e.(*ast.CallExpr); ok {
		for _, arg := range callExpr.Args {
			checkCandidate(candidates, arg)
		}
		if fun, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
			checkCandidate(candidates, fun.X)
		}
	}
}
