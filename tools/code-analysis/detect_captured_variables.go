package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
)

func main() {
	// Get the file path from the FILENAME environment variable
	filePath := os.Getenv("FILENAME")
	if filePath == "" {
		fmt.Println("FILENAME environment variable is not set.")
		os.Exit(1)
	}

	// Parse the source code
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Create a visitor function
	ast.Inspect(node, func(n ast.Node) bool {
		// Check if node is a function literal (closure)
		funcLit, ok := n.(*ast.FuncLit)
		if !ok {
			return true
		}

		// Track variables declared inside the closure
		declaredVars := make(map[string]bool)

		ast.Inspect(funcLit, func(nn ast.Node) bool {
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

		// Check for captured variables
		ast.Inspect(funcLit, func(nn ast.Node) bool {
			ident, ok := nn.(*ast.Ident)
			if !ok {
				return true
			}

			if ident.Obj == nil || ident.Obj.Kind != ast.Var {
				return true
			}

			// Check if the variable is a parameter of the function literal
			for _, param := range funcLit.Type.Params.List {
				for _, name := range param.Names {
					if name.Name == ident.Name {
						return true // Skip parameters, they are not captured variables
					}
				}
			}

			if _, isDeclared := declaredVars[ident.Name]; !isDeclared {
				pos := fset.Position(ident.Pos())
				fmt.Printf("Variable '%s' captured by closure at %s:%d\n", ident.Name, pos.Filename, pos.Line)
			}

			return true
		})

		return true
	})
}
