// Package consensus defines how blocks are produced and headers authorized.
// v0 ships a single-authority validator; the interface is shaped to grow into
// Proof-of-Storage / multi-validator BFT later.
package consensus

import (
	"errors"

	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/types"
)

// Engine decides block production rights and authorizes headers.
type Engine interface {
	// ValidatorFor returns the address allowed to produce the block at height.
	ValidatorFor(height uint64) crypto.Address
	// AuthorizeHeader checks that a header was produced by an allowed validator.
	AuthorizeHeader(h *types.Header) error
	// CanProduce reports whether the local node may produce the next block.
	CanProduce(nextHeight uint64) bool
}

// ErrUnauthorizedValidator is returned for headers from non-validator keys.
var ErrUnauthorizedValidator = errors.New("consensus: header signed by unauthorized validator")

// Authority is the v0 engine: a fixed validator set; a single producer.
type Authority struct {
	// Validators is the set of addresses allowed to sign headers.
	Validators map[crypto.Address]bool
	// Self is this node's address (empty if not a validator).
	Self crypto.Address
}

// NewAuthority builds an authority engine. self may be "" for non-producers.
func NewAuthority(validators []crypto.Address, self crypto.Address) *Authority {
	set := make(map[crypto.Address]bool, len(validators))
	for _, v := range validators {
		set[v] = true
	}
	return &Authority{Validators: set, Self: self}
}

// ValidatorFor returns Self in v0 (single producer); height is ignored.
func (a *Authority) ValidatorFor(uint64) crypto.Address { return a.Self }

// AuthorizeHeader verifies the signature and validator-set membership.
func (a *Authority) AuthorizeHeader(h *types.Header) error {
	if err := h.VerifySig(); err != nil {
		return err
	}
	if !a.Validators[h.Validator] {
		return ErrUnauthorizedValidator
	}
	return nil
}

// CanProduce is true when this node is a configured validator.
func (a *Authority) CanProduce(uint64) bool {
	return a.Self != "" && a.Validators[a.Self]
}
