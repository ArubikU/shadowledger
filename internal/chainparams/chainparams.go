// Package chainparams holds the network parameters BAKED INTO the binary, the
// way Bitcoin and Ethereum ship their genesis + DNS seeds in source. A user
// running `slnode` with no config connects to mainnet automatically: same
// genesis (derived deterministically), same validators, same bootstrap seeds.
//
// The genesis funding here is PUBLIC by design — exactly like Satoshi's genesis
// coinbase is visible in Bitcoin's source. What stays secret is the founder
// PRIVATE KEY (in secret/founder.tok), never this file.
package chainparams

import "github.com/ArubikU/shadowledger/internal/crypto"

// Params is a complete network definition.
type Params struct {
	Network     string
	Genesis     map[crypto.Address]uint64 // premine: address -> base units (counts toward 21M cap)
	Validators  []crypto.Address          // authorized block producers (v0 authority)
	Seeds       []string                  // bootstrap node control URLs
	DNSSeeds    []string                  // DNS seed hostnames (A/AAAA -> live node IPs)
	ControlAddr string                    // default control/RPC bind
	ShardAddr   string                    // default shard-transfer bind
	BlockTimeMS int
}

// Founder/treasury address — block authority + genesis premine recipient.
// PUBLIC (its private key lives only in secret/founder.tok and on the validator).
const founder crypto.Address = "sl0fa7cc3eacafa2ab32381ac11b4836d31229ddf2"

// Mainnet is the default ShadowLedger network embedded in the binary.
func Mainnet() Params {
	return Params{
		Network: "shadowledger-mainnet",
		Genesis: map[crypto.Address]uint64{
			founder: 100_000_000_000_000, // 1,000,000 SHARD treasury premine
		},
		Validators: []crypto.Address{founder},
		Seeds: []string{
			"http://136.248.77.107:4004", // bootstrap node (Oracle Cloud, sa-saopaulo-1)
		},
		DNSSeeds:    []string{},
		ControlAddr: ":4004",
		ShardAddr:   ":4005",
		BlockTimeMS: 5000,
	}
}
