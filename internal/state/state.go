// Package state holds the active account ledger: balances and nonces. This is
// the data every node keeps in full; block history is what gets sharded away.
package state

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strconv"
	"sync"

	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/economy"
	"github.com/ArubikU/shadowledger/internal/types"
	"github.com/ArubikU/shadowledger/internal/vm"
)

// Account is the per-address state. Contract accounts additionally carry Code
// and Storage; plain accounts leave them empty.
type Account struct {
	Balance uint64            `json:"balance"`
	Nonce   uint64            `json:"nonce"`
	Code    []byte            `json:"code,omitempty"`    // contract bytecode (empty for EOAs)
	Storage map[string]uint64 `json:"storage,omitempty"` // contract key/value store
}

// clone deep-copies an account (for block-level rollback snapshots).
func (a *Account) clone() *Account {
	cp := &Account{Balance: a.Balance, Nonce: a.Nonce}
	if a.Code != nil {
		cp.Code = append([]byte(nil), a.Code...)
	}
	if a.Storage != nil {
		cp.Storage = make(map[string]uint64, len(a.Storage))
		for k, v := range a.Storage {
			cp.Storage[k] = v
		}
	}
	return cp
}

// IsContract reports whether an address holds contract code.
func (s *State) IsContract(a crypto.Address) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ac := s.Accounts[a]
	return ac != nil && len(ac.Code) > 0
}

// Validator is an on-chain registered block producer. Registration locks a Bond
// (skin-in-the-game / sybil resistance); the bond is returned on unregister and
// (future) slashed for failed storage proofs or equivocation.
type Validator struct {
	Bond   uint64 `json:"bond"`
	Active bool   `json:"active"`
	Since  uint64 `json:"since"` // height registered
}

// State is a thread-safe account ledger bound to a head height.
type State struct {
	mu         sync.RWMutex
	Accounts   map[crypto.Address]*Account   `json:"accounts"`
	Validators map[crypto.Address]*Validator `json:"validators"` // on-chain validator registry
	Height     uint64                        `json:"height"`     // height of last applied block
	Minted     uint64                        `json:"minted"`     // total $SHARD emitted (counts toward cap)
}

// New returns an empty state.
func New() *State {
	return &State{
		Accounts:   make(map[crypto.Address]*Account),
		Validators: make(map[crypto.Address]*Validator),
	}
}

// RegisterGenesisValidator seats a validator at genesis with no bond (bootstrap
// authority). Used by chain.Genesis to seed the registry.
func (s *State) RegisterGenesisValidator(a crypto.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Validators == nil {
		s.Validators = make(map[crypto.Address]*Validator)
	}
	s.Validators[a] = &Validator{Bond: 0, Active: true, Since: 0}
}

// ActiveValidators returns the registered active validators, sorted.
func (s *State) ActiveValidators() []crypto.Address {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]crypto.Address, 0, len(s.Validators))
	for a, v := range s.Validators {
		if v.Active {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ValidatorInfo returns a copy of a validator record (ok=false if absent).
func (s *State) ValidatorInfo(a crypto.Address) (Validator, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v := s.Validators[a]; v != nil {
		return *v, true
	}
	return Validator{}, false
}

func (s *State) cloneValidators() map[crypto.Address]*Validator {
	cp := make(map[crypto.Address]*Validator, len(s.Validators))
	for a, v := range s.Validators {
		vc := *v
		cp[a] = &vc
	}
	return cp
}

// Supply returns the total $SHARD minted so far.
func (s *State) Supply() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Minted
}

// NextReward returns the subsidy the next block would mint at the given height.
func (s *State) NextReward(height uint64) uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return economy.BlockReward(height, s.Minted)
}

// SetMinted overrides the minted total (used by genesis bootstrap / sync).
func (s *State) SetMinted(v uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Minted = v
}

// ReplaceWith copies another state's contents in place (used by sync), keeping
// this pointer valid for the mempool and chain that already reference it.
func (s *State) ReplaceWith(other *State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Accounts = make(map[crypto.Address]*Account, len(other.Accounts))
	for a, ac := range other.Accounts {
		s.Accounts[a] = ac.clone()
	}
	s.Validators = make(map[crypto.Address]*Validator, len(other.Validators))
	for a, v := range other.Validators {
		vc := *v
		s.Validators[a] = &vc
	}
	s.Height = other.Height
	s.Minted = other.Minted
}

func (s *State) acct(a crypto.Address) *Account {
	ac := s.Accounts[a]
	if ac == nil {
		ac = &Account{}
		s.Accounts[a] = ac
	}
	return ac
}

// Get returns a copy of an account's state.
func (s *State) Get(a crypto.Address) Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if ac := s.Accounts[a]; ac != nil {
		return *ac
	}
	return Account{}
}

// Credit adds funds to an address (used for genesis funding).
func (s *State) Credit(a crypto.Address, amount uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acct(a).Balance += amount
}

// Validation errors.
var (
	ErrBadNonce         = errors.New("state: bad nonce")
	ErrInsufficient     = errors.New("state: insufficient balance")
	ErrSelfPay          = errors.New("state: from == to")
	ErrCoinbaseInBody   = errors.New("state: coinbase tx not allowed in block body")
	ErrBadCoinbase      = errors.New("state: coinbase amount/recipient violates emission schedule")
	ErrBondTooLow       = errors.New("state: validator bond below minimum")
	ErrAlreadyValidator = errors.New("state: already a registered validator")
	ErrNotValidator     = errors.New("state: not a registered validator")
)

// CheckTx validates a single tx against current state without applying it.
func (s *State) CheckTx(t *types.Transaction) error {
	if err := t.VerifySig(); err != nil {
		return err
	}
	if t.IsCoinbase() {
		return ErrCoinbaseInBody
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ac := s.Accounts[t.From]
	var bal, nonce uint64
	if ac != nil {
		bal, nonce = ac.Balance, ac.Nonce
	}
	if t.Nonce != nonce {
		return ErrBadNonce
	}
	total := t.Amount + t.Fee
	if total < t.Amount { // overflow
		return ErrInsufficient
	}
	if bal < total {
		return ErrInsufficient
	}
	return nil
}

// ApplyBlock applies all txs in order, crediting fees to the validator. It is
// atomic: on any error nothing is mutated.
func (s *State) ApplyBlock(b *types.Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Full-account snapshot for block-level rollback (covers balance, nonce,
	// code and storage, so contract effects revert cleanly too).
	saved := map[crypto.Address]*Account{} // nil value => account did not exist
	snap := func(a crypto.Address) {
		if _, ok := saved[a]; ok {
			return
		}
		if ac := s.Accounts[a]; ac != nil {
			saved[a] = ac.clone()
		} else {
			saved[a] = nil
		}
	}
	valBackup := s.cloneValidators()
	rollback := func() {
		for a, ac := range saved {
			if ac != nil {
				s.Accounts[a] = ac
			} else {
				delete(s.Accounts, a)
			}
		}
		s.Validators = valBackup
	}

	// A block may carry exactly one coinbase reward tx as Txs[0]. It mints the
	// block subsidy + collects fees, paid to the block's validator.
	var coinbase *types.Transaction
	payload := b.Txs
	if len(payload) > 0 && payload[0].IsCoinbase() {
		coinbase = &payload[0]
		payload = payload[1:]
	}

	var fees uint64
	for i := range payload {
		t := &payload[i]
		if t.IsCoinbase() {
			rollback()
			return ErrCoinbaseInBody // coinbase only allowed at index 0
		}
		if err := t.VerifySig(); err != nil {
			rollback()
			return err
		}
		from := s.acct(t.From)
		snap(t.From)
		if t.Nonce != from.Nonce {
			rollback()
			return ErrBadNonce
		}
		total := t.Amount + t.Fee
		if total < t.Amount || from.Balance < total {
			rollback()
			return ErrInsufficient
		}
		// Debit value+fee and bump nonce up front; value may be refunded on a
		// contract revert below. Fee is always kept (recycled to validator).
		from.Balance -= total
		from.Nonce++
		fees += t.Fee

		switch t.Kind {
		case types.KindDeploy:
			caddr := types.ContractAddress(t.From, t.Nonce)
			snap(caddr)
			c := s.acct(caddr)
			c.Code = append([]byte(nil), t.Data...)
			if c.Storage == nil {
				c.Storage = map[string]uint64{}
			}
			c.Balance += t.Amount
			_ = caddr // contract created; no constructor execution in v0.4
		case types.KindCall:
			snap(t.To)
			to := s.acct(t.To)
			to.Balance += t.Amount
			if len(to.Code) == 0 {
				break // calling a non-contract behaves as a plain transfer
			}
			if _, _, ok := s.execContract(t.To, types.AddrDigest(t.From), t.Data, t.Amount, t.Gas, snap, 0); !ok {
				to.Balance -= t.Amount             // revert value credit
				s.acct(t.From).Balance += t.Amount // refund sender (fee still kept)
			}
		case types.KindRegister:
			// Amount is the bond (already debited above); lock it in the registry.
			if t.Amount < economy.MinBond {
				rollback()
				return ErrBondTooLow
			}
			if v := s.Validators[t.From]; v != nil && v.Active {
				rollback()
				return ErrAlreadyValidator
			}
			s.Validators[t.From] = &Validator{Bond: t.Amount, Active: true, Since: b.Header.Height}
		case types.KindUnregister:
			v := s.Validators[t.From]
			if v == nil || !v.Active {
				rollback()
				return ErrNotValidator
			}
			// Return the bond; the value field (t.Amount) must be zero here.
			s.acct(t.From).Balance += v.Bond + t.Amount
			delete(s.Validators, t.From)
		default: // KindTransfer
			snap(t.To)
			s.acct(t.To).Balance += t.Amount
		}
	}

	// Validate and apply the coinbase against the emission schedule.
	reward := economy.BlockReward(b.Header.Height, s.Minted)
	if coinbase != nil {
		if coinbase.To != b.Header.Validator {
			rollback()
			return ErrBadCoinbase
		}
		if coinbase.Amount != reward+fees {
			rollback()
			return ErrBadCoinbase
		}
		snap(coinbase.To)
		s.acct(coinbase.To).Balance += coinbase.Amount
		s.Minted += reward // fees are recycled, not newly minted
	} else if reward+fees > 0 && b.Header.Validator != "" {
		// No explicit coinbase: still pay fees (no subsidy minted).
		snap(b.Header.Validator)
		s.acct(b.Header.Validator).Balance += fees
	}
	s.Height = b.Header.Height
	return nil
}

// storageDecode/Encode convert between the persisted string-keyed map and the
// uint64-keyed map the VM operates on.
func storageDecode(m map[string]uint64) map[uint64]uint64 {
	out := make(map[uint64]uint64, len(m))
	for k, v := range m {
		if n, err := strconv.ParseUint(k, 10, 64); err == nil {
			out[n] = v
		}
	}
	return out
}
func storageEncode(m map[uint64]uint64) map[string]uint64 {
	out := make(map[string]uint64, len(m))
	for k, v := range m {
		out[strconv.FormatUint(k, 10)] = v
	}
	return out
}

// contractByDigest resolves a VM uint64 contract id back to its address.
func (s *State) contractByDigest(d uint64) (crypto.Address, bool) {
	for addr, ac := range s.Accounts {
		if len(ac.Code) > 0 && types.AddrDigest(addr) == d {
			return addr, true
		}
	}
	return "", false
}

// execContract runs the contract at addr with the given caller, input, value
// and gas. On success it commits the contract's storage and returns its RETURN
// value; on revert it leaves storage untouched and reports ok=false. The caller
// (or vm host, for nested calls) is responsible for value transfer/refund. snap
// records accounts for block-level rollback. depth bounds recursion.
//
// Must be called with s.mu held (ApplyBlock holds it).
func (s *State) execContract(addr crypto.Address, caller uint64, input []byte, value, gas uint64, snap func(crypto.Address), depth int) (ret, used uint64, ok bool) {
	c := s.acct(addr)
	snap(addr)
	storage := storageDecode(c.Storage)
	ctx := vm.Context{
		Caller:  caller,
		Self:    types.AddrDigest(addr),
		Value:   value,
		Balance: c.Balance,
		Host:    &vmHost{s: s, snap: snap},
		Depth:   depth,
	}
	res, err := vm.Execute(c.Code, input, storage, gas, ctx)
	if err != nil {
		return 0, gas, false // reverted; storage (a copy in Execute) discarded
	}
	c.Storage = storageEncode(storage)
	return res.Return, res.GasUsed, true
}

// vmHost implements vm.CallHost so a contract can call another contract.
type vmHost struct {
	s    *State
	snap func(crypto.Address)
}

// Call resolves target to a contract and executes it (no value transfer in v0.5).
func (h *vmHost) Call(self, target, arg, gasLimit uint64, depth int) (uint64, uint64, bool) {
	addr, found := h.s.contractByDigest(target)
	if !found {
		return 0, 0, false
	}
	ret, used, ok := h.s.execContract(addr, self, u64word(arg), 0, gasLimit, h.snap, depth)
	if !ok {
		return 0, gasLimit, false
	}
	return ret, used, true
}

// u64word encodes a single uint64 as an 8-byte calldata input.
func u64word(v uint64) []byte {
	return []byte{
		byte(v >> 56), byte(v >> 48), byte(v >> 40), byte(v >> 32),
		byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v),
	}
}

// Save persists the state snapshot to disk.
func (s *State) Save(path string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// Load reads a state snapshot from disk.
func Load(path string) (*State, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := New()
	if err := json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	if s.Accounts == nil {
		s.Accounts = make(map[crypto.Address]*Account)
	}
	if s.Validators == nil {
		s.Validators = make(map[crypto.Address]*Validator)
	}
	return s, nil
}
