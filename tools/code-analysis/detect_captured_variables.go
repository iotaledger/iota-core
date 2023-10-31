package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
)

func main() {
	filePath := os.Getenv("FILENAME")
	if filePath == "" {
		fmt.Println("FILENAME environment variable is not set.")
		os.Exit(1)
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	var currentMethodReceiver string

	ast.Inspect(node, func(n ast.Node) bool {
		switch t := n.(type) {

		case *ast.FuncDecl:
			if t.Recv != nil { // This is a method
				for _, field := range t.Recv.List {
					if len(field.Names) > 0 {
						currentMethodReceiver = field.Names[0].Name
					}
				}
			} else {
				currentMethodReceiver = ""
			}

		case *ast.FuncLit:
			declaredVars := make(map[string]bool)

			ast.Inspect(t, func(nn ast.Node) bool {
				switch innerNode := nn.(type) {
				case *ast.AssignStmt:
					for _, expr := range innerNode.Lhs {
						if ident, ok := expr.(*ast.Ident); ok {
							declaredVars[ident.Name] = true
						}
					}
				case *ast.ValueSpec:
					for _, ident := range innerNode.Names {
						declaredVars[ident.Name] = true
					}
				}
				return true
			})

			ast.Inspect(t, func(nn ast.Node) bool {
				ident, ok := nn.(*ast.Ident)
				if !ok {
					return true
				}

				if ident.Name == "_" { // skip underscore variables
					return true
				}

				if ident.Obj == nil || ident.Obj.Kind != ast.Var {
					return true
				}

				for _, param := range t.Type.Params.List {
					for _, name := range param.Names {
						if name.Name == ident.Name {
							return true
						}
					}
				}

				if _, isDeclared := declaredVars[ident.Name]; !isDeclared {
					if ident.Name != currentMethodReceiver {
						pos := fset.Position(ident.Pos())
						fmt.Printf("%s:%d: variable '%s' captured by closure\n", pos.Filename, pos.Line, ident.Name)
					}
				}
				return true
			})
		}
		return true
	})
}
