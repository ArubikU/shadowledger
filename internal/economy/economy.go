// Package economy defines $SHARD monetary policy: a fixed supply cap with a
// Bitcoin-style halving emission schedule.
//
// Crucially this is DECOUPLED from consensus: the block reward is minted to
// whoever produced the block (the validator), not to a Proof-of-Work miner.
// ShadowLedger keeps Bitcoin's scarcity model (cap + halving) WITHOUT Bitcoin's
// energy waste — see docs/GAPS-AND-DESIGN.md.
package economy

// Base-unit constants (mirroring Bitcoin's 1 BTC = 100,000,000 sats).
const (
	Decimals = 8
	// Coin is the number of base units in one whole SHARD.
	Coin uint64 = 100_000_000
	// MaxSupply caps total emission at 21,000,000 SHARD.
	MaxSupply uint64 = 21_000_000 * Coin
	// InitialReward is the block subsidy before the first halving (50 SHARD).
	InitialReward uint64 = 50 * Coin
	// HalvingInterval halves the subsidy every this many blocks.
	HalvingInterval uint64 = 210_000
	// MinBond is the minimum $SHARD a node must lock to register as a validator
	// (sybil resistance / skin-in-the-game). 1,000 SHARD.
	MinBond uint64 = 1_000 * Coin
)

// BlockReward returns the subsidy minted for producing the block at `height`,
// given the amount already minted. It halves every HalvingInterval blocks and
// is clamped so total emission never exceeds MaxSupply.
//
// Note: genesis premine counts toward `minted`, so it is part of the 21M cap.
func BlockReward(height, minted uint64) uint64 {
	if minted >= MaxSupply {
		return 0
	}
	halvings := height / HalvingInterval
	if halvings >= 64 { // subsidy has shifted to zero
		return 0
	}
	reward := InitialReward >> halvings
	if remaining := MaxSupply - minted; reward > remaining {
		reward = remaining
	}
	return reward
}
