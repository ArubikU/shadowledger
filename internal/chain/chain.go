// Package chain wires together state, storage, erasure coding and shard
// placement into an append-only ledger.
//
// Propagation model (v0): a freshly produced block travels in full on the
// control channel so every node can apply its state transition. After applying,
// a node persists ONLY the erasure shards it is assigned by rendezvous hashing
// and discards the full body. Old block bodies therefore live nowhere whole —
// they are reconstructed on demand from K-of-(K+M) scattered shards.
package chain

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/ArubikU/shadowledger/internal/bloom"
	"github.com/ArubikU/shadowledger/internal/consensus"
	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/erasure"
	"github.com/ArubikU/shadowledger/internal/netparams"
	"github.com/ArubikU/shadowledger/internal/rendezvous"
	"github.com/ArubikU/shadowledger/internal/state"
	"github.com/ArubikU/shadowledger/internal/store"
	"github.com/ArubikU/shadowledger/internal/types"
)

// ShardSource fetches a remote shard for reconstruction (implemented by p2p).
type ShardSource interface {
	FetchShard(blockID types.Hash, index int, set types.ShardSet) ([]byte, error)
}

// Chain is the local view of the ledger.
type Chain struct {
	mu      sync.RWMutex
	store   *store.Store
	state   *state.State
	engine  consensus.Engine
	bloom   *bloom.Filter
	selfID  string
	members func() []string // current node-id set for rendezvous

	head   types.Header
	hasGen bool
	source ShardSource
}

// Config parameters for a chain. Erasure shape and replication are NOT set
// here — they are derived from the live node count by package netparams.
type Config struct {
	SelfID  string
	Members func() []string
}

// New builds a chain over an opened store and state.
func New(st *store.Store, st2 *state.State, eng consensus.Engine, bf *bloom.Filter, cfg Config) *Chain {
	c := &Chain{
		store: st, state: st2, engine: eng, bloom: bf,
		selfID: cfg.SelfID, members: cfg.Members,
	}
	if c.store.HasHeaders() {
		if hdr, _, err := c.store.GetHeader(c.store.Height()); err == nil {
			c.head = hdr
			c.hasGen = true
		}
	}
	return c
}

// replFor returns the replication factor the network policy picks for the
// current membership size.
func (c *Chain) replFor() int { return netparams.Replication(len(c.members())) }

// specFor returns the erasure (K,M) the network policy picks for the current
// membership size. Used when PRODUCING a block; consumers read header.Spec.
func (c *Chain) specFor() types.ShardSpec { return netparams.Spec(len(c.members())) }

// SetSource attaches the remote shard fetcher used during reconstruction.
func (c *Chain) SetSource(s ShardSource) { c.source = s }

// Errors.
var (
	ErrNoGenesis   = errors.New("chain: no genesis block")
	ErrHasGenesis  = errors.New("chain: genesis already exists")
	ErrBadHeight   = errors.New("chain: unexpected block height")
	ErrBadPrev     = errors.New("chain: prev hash mismatch")
	ErrBadMerkle   = errors.New("chain: merkle root mismatch")
	ErrBadBodyHash = errors.New("chain: body hash mismatch")
	ErrReconstruct = errors.New("chain: could not reconstruct block body")
)

// Head returns the current head header.
func (c *Chain) Head() (types.Header, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.head, c.hasGen
}

// Height returns the current head height (0 if no genesis).
func (c *Chain) Height() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.head.Height
}

// HeadID returns the current head block id (zero hash before genesis).
func (c *Chain) HeadID() types.Hash {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.hasGen {
		return types.Hash{}
	}
	return c.head.ID()
}

// State exposes the account ledger.
func (c *Chain) State() *state.State { return c.state }

// Spec returns the erasure shape the network policy currently picks.
func (c *Chain) Spec() types.ShardSpec { return c.specFor() }

// Genesis creates and applies block 0 from a funding table. It is fully
// deterministic — fixed timestamp, sorted funding order, fixed validator stamp —
// so every node independently derives an IDENTICAL genesis block (and thus the
// same prev-hash chain) from the same config, without needing it gossiped. The
// genesis header carries a valid signature only when kp matches genesisVal.
func (c *Chain) Genesis(funding map[crypto.Address]uint64, genesisVal crypto.Address, kp *crypto.KeyPair) (*types.Block, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hasGen {
		return nil, ErrHasGenesis
	}
	// Deterministic ordering: sort funded addresses.
	addrs := make([]crypto.Address, 0, len(funding))
	for a := range funding {
		addrs = append(addrs, a)
	}
	sort.Slice(addrs, func(i, j int) bool { return addrs[i] < addrs[j] })
	txs := make([]types.Transaction, 0, len(addrs))
	for _, a := range addrs {
		txs = append(txs, types.Transaction{To: a, Amount: funding[a]})
	}
	blk := &types.Block{Txs: txs}
	body := blk.Body()
	hdr := types.Header{
		Height:     0,
		PrevHash:   types.Hash{},
		MerkleRoot: types.MerkleRootOf(txs),
		Timestamp:  0, // fixed for determinism across nodes
		Validator:  genesisVal,
		TxCount:    uint32(len(txs)),
		Spec:       netparams.GenesisSpec(), // fixed shape -> identical genesis everywhere
		BodyHash:   types.BodyHash(body),
	}
	if kp != nil && kp.Address() == genesisVal {
		hdr.Sign(kp) // producer signs; followers leave genesis sig empty
	}
	blk.Header = hdr

	// Apply funding directly (coinbase txs skip the body-coinbase ban). Genesis
	// premine counts toward the 21M $SHARD cap.
	var premine uint64
	for addr, amt := range funding {
		c.state.Credit(addr, amt)
		premine += amt
	}
	c.state.SetMinted(premine)
	c.state.RegisterGenesisValidator(genesisVal) // seed the on-chain validator registry
	c.state.Height = 0

	if err := c.persistBlock(&hdr, body); err != nil {
		return nil, err
	}
	c.head = hdr
	c.hasGen = true
	return blk, nil
}

// ProduceBlock builds, signs, persists and locally shards a new block from txs.
// Returns the full block (for gossip) and its shard set.
func (c *Chain) ProduceBlock(txs []types.Transaction, kp *crypto.KeyPair) (*types.Block, types.ShardSet, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hasGen {
		return nil, types.ShardSet{}, ErrNoGenesis
	}
	prev := c.head
	height := prev.Height + 1

	// Prepend the coinbase reward tx: block subsidy (emission schedule) + fees,
	// paid to this validator. No PoW — the producer simply earns the subsidy.
	var fees uint64
	for i := range txs {
		fees += txs[i].Fee
	}
	reward := c.state.NextReward(height)
	full := make([]types.Transaction, 0, len(txs)+1)
	if reward+fees > 0 {
		full = append(full, types.Transaction{To: kp.Address(), Amount: reward + fees})
	}
	full = append(full, txs...)

	blk := &types.Block{Txs: full}
	body := blk.Body()
	hdr := types.Header{
		Height:     height,
		PrevHash:   prev.ID(),
		MerkleRoot: types.MerkleRootOf(full),
		Timestamp:  time.Now().Unix(),
		TxCount:    uint32(len(full)),
		Spec:       c.specFor(), // network-decided erasure shape for this block
		BodyHash:   types.BodyHash(body),
	}
	hdr.Sign(kp)
	blk.Header = hdr

	if err := c.state.ApplyBlock(blk); err != nil {
		return nil, types.ShardSet{}, err
	}
	set, err := c.persistBlockSet(&hdr, body)
	if err != nil {
		return nil, types.ShardSet{}, err
	}
	_ = c.store.PutLogs(hdr.Height, c.state.LastLogs())
	c.head = hdr
	return blk, set, nil
}

// ApplyExternalBlock validates and applies a block received from a peer.
func (c *Chain) ApplyExternalBlock(blk *types.Block) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hasGen {
		return ErrNoGenesis
	}
	hdr := &blk.Header
	if err := c.engine.AuthorizeHeader(hdr); err != nil {
		return err
	}
	if hdr.Height != c.head.Height+1 {
		return ErrBadHeight
	}
	if hdr.PrevHash != c.head.ID() {
		return ErrBadPrev
	}
	if types.MerkleRootOf(blk.Txs) != hdr.MerkleRoot {
		return ErrBadMerkle
	}
	body := blk.Body()
	if types.BodyHash(body) != hdr.BodyHash {
		return ErrBadBodyHash
	}
	if err := c.state.ApplyBlock(blk); err != nil {
		return err
	}
	if _, err := c.persistBlockSet(hdr, body); err != nil {
		return err
	}
	_ = c.store.PutLogs(hdr.Height, c.state.LastLogs())
	c.head = *hdr
	return nil
}

// Logs returns the raw JSON event logs stored at a height.
func (c *Chain) Logs(height uint64) []byte { return c.store.GetLogs(height) }

// persistBlock stores the header (computing the shard set) without keeping body.
func (c *Chain) persistBlock(hdr *types.Header, body []byte) error {
	_, err := c.persistBlockSet(hdr, body)
	return err
}

// persistBlockSet erasure-codes body, commits the shard set, persists the header
// and stores only the shards this node is assigned by rendezvous.
func (c *Chain) persistBlockSet(hdr *types.Header, body []byte) (types.ShardSet, error) {
	blockID := hdr.ID()
	shards, set, err := erasure.BuildShardSet(blockID, body, hdr.Spec)
	if err != nil {
		return types.ShardSet{}, err
	}
	if err := c.store.PutHeader(*hdr, set); err != nil {
		return types.ShardSet{}, err
	}
	c.storeAssignedShards(blockID, shards, set)
	return set, nil
}

// storeAssignedShards persists the shards for which this node ranks in top-R,
// where R is the network-decided replication factor for the current node count.
func (c *Chain) storeAssignedShards(blockID types.Hash, shards [][]byte, set types.ShardSet) {
	members := c.members()
	repl := netparams.Replication(len(members))
	for i, data := range shards {
		if rendezvous.IsHolder(c.selfID, members, blockID, i, repl) {
			_ = c.store.PutShard(blockID, i, data)
			if c.bloom != nil {
				c.bloom.Add(set.ShardHash[i])
			}
		}
	}
}

// Members returns the current rendezvous membership set.
func (c *Chain) Members() []string { return c.members() }

// ReconstructBody rematerializes a block body at the given height from local
// shards plus, if needed, shards fetched from peers (the slow path). It returns
// the decoded transactions.
func (c *Chain) ReconstructBody(height uint64) ([]types.Transaction, error) {
	hdr, set, err := c.store.GetHeader(height)
	if err != nil {
		return nil, err
	}
	total := set.Spec.Total()
	shards := make([][]byte, total)
	have := 0
	blockID := hdr.ID()

	// Local shards first.
	for _, i := range c.store.HeldShards(blockID) {
		if i >= 0 && i < total {
			if data, err := c.store.GetShard(blockID, i); err == nil && erasure.VerifyShard(set, i, data) {
				shards[i] = data
				have++
			}
		}
	}
	// Fetch missing shards until we reach K valid ones.
	if have < int(set.Spec.K) && c.source != nil {
		for i := 0; i < total && have < int(set.Spec.K); i++ {
			if shards[i] != nil {
				continue
			}
			data, ferr := c.source.FetchShard(blockID, i, set)
			if ferr != nil || data == nil {
				continue
			}
			if !erasure.VerifyShard(set, i, data) {
				continue // corrupt / lying peer; skip
			}
			shards[i] = data
			have++
		}
	}
	if have < int(set.Spec.K) {
		return nil, ErrReconstruct
	}
	body, err := erasure.Reconstruct(shards, set.Spec, set.OrigLen)
	if err != nil {
		return nil, err
	}
	return types.DecodeTxs(body)
}

// HeaderAt returns the stored header + shard set at height.
func (c *Chain) HeaderAt(height uint64) (types.Header, types.ShardSet, error) {
	return c.store.GetHeader(height)
}

// Store exposes the underlying store (for shard serving).
func (c *Chain) Store() *store.Store { return c.store }

// HeaderRecord pairs a header with its shard-set commitment (sync transport).
type HeaderRecord struct {
	Header   types.Header   `json:"header"`
	ShardSet types.ShardSet `json:"shard_set"`
}

// SyncInstall fast-syncs a freshly joined node from a verified header chain plus
// a trusted account-state snapshot taken at the chain tip.
//
// Trust model (v0): the header chain is fully VERIFIED — every header's
// validator signature is checked and each links to the prior via PrevHash, so a
// joiner cannot be fed a forged history. The state snapshot itself is TRUSTED
// from the serving peer (assumeutxo-style); a fully trustless joiner would
// instead reconstruct every block body from shards and replay. A future header
// `StateRoot` commitment would let the snapshot be verified directly.
func (c *Chain) SyncInstall(records []HeaderRecord, snap *state.State) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(records) == 0 {
		return ErrReconstruct
	}
	var prevID types.Hash
	for i, rec := range records {
		h := rec.Header
		if h.Height != uint64(i) {
			return ErrBadHeight
		}
		if i == 0 {
			if h.PrevHash != (types.Hash{}) {
				return ErrBadPrev
			}
		} else if h.PrevHash != prevID {
			return ErrBadPrev
		}
		if err := c.engine.AuthorizeHeader(&h); err != nil {
			return err
		}
		if err := c.store.PutHeader(h, rec.ShardSet); err != nil {
			return err
		}
		prevID = h.ID()
	}
	c.state.ReplaceWith(snap)
	c.head = records[len(records)-1].Header
	c.hasGen = true
	return nil
}
