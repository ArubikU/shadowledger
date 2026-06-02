package types

import "errors"

// Encoding errors.
var (
	ErrBadBody = errors.New("types: malformed block body")
)
