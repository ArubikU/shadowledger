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
	Network        string
	ChainID        uint64                    // network id bound into every tx (cross-network replay protection)
	GenesisTime    int64                     // fixed genesis timestamp (anchors round timing; same on all nodes)
	RegPoWBits     int                       // validator-registration PoW difficulty (Sybil gate; 0 = off)
	Consensus      string                    // "authority" | "postorage"
	FinalityDepth  int                       // blocks behind head that become irreversible (0 = off)
	FaucetAmount   uint64                    // $SHARD paid per successful PoW faucet claim (0 = faucet off)
	FaucetBits     int                       // faucet PoW difficulty (leading zero bits)
	FaucetCooldown uint64                    // blocks an address must wait between faucet claims
	FaucetDepth    uint64                    // min blocks the PoW anchor must be behind head (confirmation)
	Genesis        map[crypto.Address]uint64 // premine: address -> base units (counts toward 21M cap)
	Validators     []crypto.Address          // authorized block producers (v0 authority)
	Seeds          []string                  // bootstrap node control URLs
	DNSSeeds       []string                  // DNS seed hostnames (A/AAAA -> live node IPs)
	ControlAddr    string                    // default control/RPC bind
	ShardAddr      string                    // default shard-transfer bind
	BlockTimeMS    int
}

// Founder/treasury address — block authority + genesis premine recipient.
// PUBLIC (its private key lives only in secret/founder.tok and on the validator).
const founder crypto.Address = "sl0fa7cc3eacafa2ab32381ac11b4836d31229ddf2"

// Mainnet is the default ShadowLedger network embedded in the binary.
func Mainnet() Params {
	return Params{
		Network:       "shadowledger-mainnet",
		ChainID:       1,
		GenesisTime:   1780000000,  // fixed launch anchor (unix); deterministic across nodes
		RegPoWBits:    20,          // ~1M hashes to register a validator (Sybil gate; sub-second)
		Consensus:     "postorage", // permissionless multi-validator with reorg fork-choice
		FinalityDepth: 16,          // blocks behind head become irreversible (no reorg below)
		// Follower on-ramp: earn a little $SHARD by PoW-mining against a recent
		// block's commitment. Paid from the treasury (founder), not newly minted.
		FaucetAmount:   200_000_000, // 2 SHARD per claim (1 SHARD = 1e8 base units)
		FaucetBits:     18,          // ~260k hashes per claim (seconds on a laptop)
		FaucetCooldown: 50,          // an address may claim once per 50 blocks
		FaucetDepth:    4,           // anchor block must be >=4 deep (confirmed)
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
