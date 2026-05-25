package astx

import (
	"fmt"
	"go/token"
)

// PosError is a positioned error from AST extraction.
type PosError struct {
	Pos token.Position
	Msg string
}

func (e *PosError) Error() string {
	return fmt.Sprintf("%s: %s", e.Pos, e.Msg)
}
