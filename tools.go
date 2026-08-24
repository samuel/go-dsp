//go:build tools

// Package tools pins the build-time dependencies of the avo generators.
//
// The generators are //go:build ignore, and go mod tidy does not look inside
// ignored files, so without this one tidy drops avo and x/tools from go.mod and
// go generate stops working until they are re-added by hand. The tools tag is
// never set by any build, so nothing here is ever compiled -- but 'go mod tidy'
// does read it.
//
// x/tools has to stay newer than avo's own pin, which does not compile under
// Go 1.27. That is why it is required explicitly rather than left to avo.
package tools

import (
	// Imported for their effect on go.mod alone; see the package comment.
	_ "github.com/mmcloughlin/avo/build"
	_ "github.com/mmcloughlin/avo/operand"
	_ "github.com/mmcloughlin/avo/reg"
	_ "golang.org/x/tools/go/packages"
)
