package chain

import (
	"encoding/json"
	"testing"

	"github.com/ArubikU/shadowledger/internal/bloom"
	"github.com/ArubikU/shadowledger/internal/consensus"
	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/economy"
	"github.com/ArubikU/shadowledger/internal/mempool"
	"github.com/ArubikU/shadowledger/internal/state"
	"github.com/ArubikU/shadowledger/internal/store"
	"github.com/ArubikU/shadowledger/internal/types"
)

// newTestChain builds a single-node chain whose own assignment covers every
// shard (members = {self}, repl >= total), so it can self-reconstruct.
func newTestChain(t *testing.T, self *crypto.KeyPair) *Chain {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := state.New()
	eng := consensus.NewAuthority([]crypto.Address{self.Address()}, self.Address())
	bf := bloom.New(1000, 0.01)
	members := func() []string { return []string{string(self.Address())} }
	// Single-member set: netparams gives repl=1 and Spec(1) (K=2,M=1); the lone
	// node ranks top-1 for every shard, so it holds all of them and can
	// self-reconstruct without peers.
	return New(st, ledger, eng, bf, Config{
		SelfID:  string(self.Address()),
		Members: members,
	})
}

func TestGenesisProduceReconstruct(t *testing.T) {
	val, _ := crypto.Generate()
	alice, _ := crypto.Generate()
	bob, _ := crypto.Generate()

	c := newTestChain(t, val)
	pool := mempool.New(c.State(), 100)

	// Genesis funds alice.
	if _, err := c.Genesis(map[crypto.Address]uint64{alice.Address(): 1_000_000}, val.Address(), val); err != nil {
		t.Fatal(err)
	}
	if got := c.State().Get(alice.Address()).Balance; got != 1_000_000 {
		t.Fatalf("alice balance after genesis = %d", got)
	}

	// Alice pays bob 250k with 10 fee.
	tx := types.Transaction{To: bob.Address(), Amount: 250_000, Fee: 10, Nonce: 0}
	tx.Sign(alice)
	if err := pool.Submit(tx); err != nil {
		t.Fatalf("submit: %v", err)
	}
	txs := pool.Reap(10)
	if len(txs) != 1 {
		t.Fatalf("reap got %d txs", len(txs))
	}
	blk, _, err := c.ProduceBlock(txs, val)
	if err != nil {
		t.Fatalf("produce: %v", err)
	}
	if blk.Header.Height != 1 {
		t.Fatalf("height = %d", blk.Header.Height)
	}

	// State checks.
	if got := c.State().Get(bob.Address()).Balance; got != 250_000 {
		t.Fatalf("bob balance = %d", got)
	}
	if got := c.State().Get(alice.Address()).Balance; got != 749_990 {
		t.Fatalf("alice balance = %d", got)
	}
	// Validator earns the block subsidy (emission schedule) + fees.
	wantVal := economy.InitialReward + 10
	if got := c.State().Get(val.Address()).Balance; got != wantVal {
		t.Fatalf("validator balance = %d, want %d", got, wantVal)
	}
	// Minted grows by exactly the subsidy (fees are recycled, not minted).
	if got := c.State().Supply(); got != 1_000_000+economy.InitialReward {
		t.Fatalf("supply = %d", got)
	}

	// Slow path: reconstruct block 1 body purely from stored shards. Block 1
	// holds the coinbase (index 0) plus alice's transfer.
	rebuilt, err := c.ReconstructBody(1)
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	if len(rebuilt) != 2 {
		t.Fatalf("reconstructed tx count = %d, want 2", len(rebuilt))
	}
	if !rebuilt[0].IsCoinbase() || rebuilt[1].Hash() != tx.Hash() {
		t.Fatalf("reconstructed body does not match expected (coinbase + transfer)")
	}
}

// TestLogsHybridRoundTrip: a contract emits an event; the logs are erasure-coded
// + stored as shards (ShadowLedger), committed via header.LogsRoot (Ethereum),
// then reconstructed and integrity-checked on read.
func TestLogsHybridRoundTrip(t *testing.T) {
	val, _ := crypto.Generate()
	c := newTestChain(t, val)
	if _, err := c.Genesis(map[crypto.Address]uint64{val.Address(): 1_000_000_000}, val.Address(), val); err != nil {
		t.Fatal(err)
	}
	// Contract: PUSH 99 ; PUSH 1 ; LOG ; STOP  (emits one event, topic=99).
	code := []byte{0x01, 0, 0, 0, 0, 0, 0, 0, 99, 0x01, 0, 0, 0, 0, 0, 0, 0, 1, 0x72, 0x00}
	dep := types.Transaction{Kind: types.KindDeploy, Data: code, Nonce: 0}
	dep.Sign(val)
	if _, _, err := c.ProduceBlock([]types.Transaction{dep}, val); err != nil {
		t.Fatalf("deploy block: %v", err)
	}
	caddr := types.ContractAddress(val.Address(), 0)

	call := types.Transaction{To: caddr, Kind: types.KindCall, Gas: 100000, Nonce: 1}
	call.Sign(val)
	blk, _, err := c.ProduceBlock([]types.Transaction{call}, val)
	if err != nil {
		t.Fatalf("call block: %v", err)
	}
	h := blk.Header.Height

	// header committed a non-zero logs root...
	if blk.Header.LogsRoot == (types.Hash{}) {
		t.Fatal("logs root not committed in header")
	}
	// ...and the logs reconstruct from shards + pass the integrity check.
	var logs []types.Log
	if err := json.Unmarshal(c.Logs(h), &logs); err != nil {
		t.Fatalf("logs json: %v", err)
	}
	if len(logs) != 1 || len(logs[0].Topics) != 1 || logs[0].Topics[0] != 99 || logs[0].Contract != caddr {
		t.Fatalf("reconstructed logs wrong: %+v", logs)
	}
}

func TestRejectBadNonce(t *testing.T) {
	val, _ := crypto.Generate()
	alice, _ := crypto.Generate()
	c := newTestChain(t, val)
	pool := mempool.New(c.State(), 100)
	c.Genesis(map[crypto.Address]uint64{alice.Address(): 100}, val.Address(), val)

	tx := types.Transaction{To: val.Address(), Amount: 1, Nonce: 5} // wrong nonce
	tx.Sign(alice)
	if err := pool.Submit(tx); err == nil {
		t.Fatal("expected bad-nonce rejection")
	}
}
