package astx

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"strings"
)

// FormatNode renders an AST node back to Go source. For *ast.BlockStmt the
// outer braces are stripped so the caller can splice the inner statements
// directly into another body.
func FormatNode(node ast.Node, fset *token.FileSet) (string, error) {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, node); err != nil {
		return "", fmt.Errorf("astx: format.Node: %w", err)
	}
	out := buf.String()

	if _, ok := node.(*ast.BlockStmt); ok {
		// Trim leading `{` and trailing `}` plus surrounding whitespace.
		out = strings.TrimSpace(out)
		if strings.HasPrefix(out, "{") {
			out = out[1:]
		}
		if strings.HasSuffix(out, "}") {
			out = out[:len(out)-1]
		}
		out = strings.Trim(out, " \t\r\n")
	}

	return out, nil
}
