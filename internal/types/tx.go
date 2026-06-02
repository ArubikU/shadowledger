// Package types defines ShadowLedger's core data structures: transactions,
// blocks, headers and shard sets, with deterministic canonical encodings.
package types

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"

	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/merkle"
)

// Hash is the project-wide 32-byte digest (shared with merkle).
type Hash = merkle.Hash

// Transaction kinds.
const (
	KindTransfer   uint8 = 0 // plain value transfer
	KindDeploy     uint8 = 1 // deploy a contract (Data = bytecode)
	KindCall       uint8 = 2 // call a contract (To = contract, Data = input words)
	KindRegister   uint8 = 3 // register as a validator, locking Amount as a bond
	KindUnregister uint8 = 4 // exit the validator set, unlocking the bond
	KindSlash      uint8 = 5 // report equivocation; Data = two conflicting signed headers
)

// EquivocationEvidence is two conflicting headers signed by the same validator
// at the same height — provable double-signing, carried in a KindSlash tx.Data.
type EquivocationEvidence struct {
	A Header `json:"a"`
	B Header `json:"b"`
}

// Transaction is an account-model value transfer or contract operation.
type Transaction struct {
	From   crypto.Address `json:"from"`
	To     crypto.Address `json:"to"`
	Amount uint64         `json:"amount"`
	Fee    uint64         `json:"fee"`
	Nonce  uint64         `json:"nonce"`
	Kind   uint8          `json:"kind"`   // 0 transfer, 1 deploy, 2 call
	Gas    uint64         `json:"gas"`    // execution gas limit (contract txs)
	Data   []byte         `json:"data"`   // deploy bytecode / call input
	PubKey []byte         `json:"pubkey"` // sender ed25519 pubkey (empty for coinbase)
	Sig    []byte         `json:"sig"`    // ed25519 over SigningBytes (empty for coinbase)
}

func putString(buf *bytes.Buffer, s string) {
	var l [8]byte
	binary.BigEndian.PutUint64(l[:], uint64(len(s)))
	buf.Write(l[:])
	buf.WriteString(s)
}

func putBytes(buf *bytes.Buffer, b []byte) {
	var l [8]byte
	binary.BigEndian.PutUint64(l[:], uint64(len(b)))
	buf.Write(l[:])
	buf.Write(b)
}

func putU64(buf *bytes.Buffer, v uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	buf.Write(b[:])
}

// SigningBytes is the canonical encoding signed by the sender (excludes Sig).
func (t *Transaction) SigningBytes() []byte {
	var b bytes.Buffer
	putString(&b, string(t.From))
	putString(&b, string(t.To))
	putU64(&b, t.Amount)
	putU64(&b, t.Fee)
	putU64(&b, t.Nonce)
	b.WriteByte(t.Kind)
	putU64(&b, t.Gas)
	putBytes(&b, t.Data)
	putBytes(&b, t.PubKey)
	return b.Bytes()
}

// ContractAddress derives a deployed contract's address from the deployer and
// the nonce used in the deploy tx (deterministic, like Ethereum's CREATE).
func ContractAddress(deployer crypto.Address, nonce uint64) crypto.Address {
	var nb [8]byte
	for i := 0; i < 8; i++ {
		nb[7-i] = byte(nonce >> (8 * i))
	}
	sum := sha256.Sum256(append([]byte("contract:"+deployer), nb[:]...))
	return crypto.Address(crypto.AddressPrefix + hex.EncodeToString(sum[:20]))
}

// AddrDigest reduces an address to a uint64 the VM can use (e.g. CALLER).
func AddrDigest(a crypto.Address) uint64 {
	h := sha256.Sum256([]byte(a))
	return binary.BigEndian.Uint64(h[:8])
}

// canonical is the full encoding including Sig, used for the tx hash.
func (t *Transaction) canonical() []byte {
	b := bytes.NewBuffer(t.SigningBytes())
	putBytes(b, t.Sig)
	return b.Bytes()
}

// Hash returns the transaction id (SHA-256 of the full canonical encoding).
func (t *Transaction) Hash() Hash {
	return sha256.Sum256(t.canonical())
}

// IsCoinbase reports whether this is a genesis/reward tx (no sender).
func (t *Transaction) IsCoinbase() bool { return t.From == "" }

// Sign fills PubKey and Sig using the given key pair and sets From accordingly.
func (t *Transaction) Sign(kp *crypto.KeyPair) {
	t.From = kp.Address()
	t.PubKey = kp.Pub
	t.Sig = kp.Sign(t.SigningBytes())
}

// VerifySig validates the signature and that the pubkey matches From.
// Coinbase txs are considered valid here (authorized separately by the chain).
func (t *Transaction) VerifySig() error {
	if t.IsCoinbase() {
		return nil
	}
	if len(t.PubKey) != ed25519.PublicKeySize {
		return ErrBadPubKey
	}
	if !crypto.PubMatchesAddress(t.PubKey, t.From) {
		return ErrPubAddrMismatch
	}
	if !crypto.Verify(t.PubKey, t.SigningBytes(), t.Sig) {
		return ErrBadSignature
	}
	return nil
}

// Transaction validation errors.
var (
	ErrBadPubKey       = errors.New("tx: bad pubkey length")
	ErrPubAddrMismatch = errors.New("tx: pubkey does not match from-address")
	ErrBadSignature    = errors.New("tx: invalid signature")
)
