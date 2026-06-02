package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"

	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/merkle"
)

// ShardSpec captures the erasure parameters used to fragment a block body.
type ShardSpec struct {
	K uint8 `json:"k"` // data shards
	M uint8 `json:"m"` // parity shards
}

// Total returns K+M, the number of shards produced.
func (s ShardSpec) Total() int { return int(s.K) + int(s.M) }

// ShardSet is the commitment to a block's erasure shards.
type ShardSet struct {
	BlockID   Hash      `json:"block_id"`
	Spec      ShardSpec `json:"spec"`
	PaddedLen int       `json:"padded_len"` // length each data shard slot was padded to
	OrigLen   int       `json:"orig_len"`   // original body length (to trim after decode)
	ShardHash []Hash    `json:"shard_hash"` // len == Spec.Total(); hash of each shard
}

// Header is the lightweight block header that gossips network-wide.
type Header struct {
	Height     uint64         `json:"height"`
	PrevHash   Hash           `json:"prev_hash"`
	MerkleRoot Hash           `json:"merkle_root"`
	Timestamp  int64          `json:"timestamp"`
	Validator  crypto.Address `json:"validator"`
	TxCount    uint32         `json:"tx_count"`
	Spec       ShardSpec      `json:"spec"`
	BodyHash   Hash           `json:"body_hash"` // SHA-256 of canonical body bytes
	LogsRoot   Hash           `json:"logs_root"` // merkle root over this block's event logs (Ethereum-style)
	Sig        []byte         `json:"sig"`       // validator ed25519 over SigningBytes
	ValPubKey  []byte         `json:"val_pubkey"`
}

// Block bundles a header with its full transaction list (the erasure payload).
type Block struct {
	Header Header        `json:"header"`
	Txs    []Transaction `json:"txs"`
}

// Body returns the canonical encoding of the tx list — the bytes that get
// erasure-coded into shards. Reconstructing these bytes reconstructs the block.
func (b *Block) Body() []byte {
	return EncodeTxs(b.Txs)
}

// EncodeTxs canonically serializes a transaction slice.
func EncodeTxs(txs []Transaction) []byte {
	var buf bytes.Buffer
	var cnt [4]byte
	binary.BigEndian.PutUint32(cnt[:], uint32(len(txs)))
	buf.Write(cnt[:])
	for i := range txs {
		putBytes(&buf, txs[i].canonical())
	}
	return buf.Bytes()
}

// DecodeTxs reverses EncodeTxs.
func DecodeTxs(data []byte) ([]Transaction, error) {
	if len(data) < 4 {
		return nil, ErrBadBody
	}
	n := binary.BigEndian.Uint32(data[:4])
	rest := data[4:]
	txs := make([]Transaction, 0, n)
	for i := uint32(0); i < n; i++ {
		if len(rest) < 8 {
			return nil, ErrBadBody
		}
		l := binary.BigEndian.Uint64(rest[:8])
		rest = rest[8:]
		if uint64(len(rest)) < l {
			return nil, ErrBadBody
		}
		tx, err := decodeTx(rest[:l])
		if err != nil {
			return nil, err
		}
		txs = append(txs, tx)
		rest = rest[l:]
	}
	return txs, nil
}

func readBytes(r *bytes.Reader) ([]byte, error) {
	var l [8]byte
	if _, err := r.Read(l[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint64(l[:])
	out := make([]byte, n)
	if n == 0 {
		return out, nil
	}
	if _, err := r.Read(out); err != nil {
		return nil, err
	}
	return out, nil
}

func readU64(r *bytes.Reader) (uint64, error) {
	var b [8]byte
	if _, err := r.Read(b[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(b[:]), nil
}

func decodeTx(data []byte) (Transaction, error) {
	r := bytes.NewReader(data)
	var t Transaction
	from, err := readBytes(r)
	if err != nil {
		return t, err
	}
	to, err := readBytes(r)
	if err != nil {
		return t, err
	}
	t.From = crypto.Address(from)
	t.To = crypto.Address(to)
	if t.Amount, err = readU64(r); err != nil {
		return t, err
	}
	if t.Fee, err = readU64(r); err != nil {
		return t, err
	}
	if t.Nonce, err = readU64(r); err != nil {
		return t, err
	}
	if t.ChainID, err = readU64(r); err != nil {
		return t, err
	}
	kind, err := r.ReadByte()
	if err != nil {
		return t, err
	}
	t.Kind = kind
	if t.Gas, err = readU64(r); err != nil {
		return t, err
	}
	if t.Data, err = readBytes(r); err != nil {
		return t, err
	}
	if t.PubKey, err = readBytes(r); err != nil {
		return t, err
	}
	if t.Sig, err = readBytes(r); err != nil {
		return t, err
	}
	return t, nil
}

// BodyHash returns the SHA-256 of the canonical body.
func BodyHash(body []byte) Hash { return sha256.Sum256(body) }

// SigningBytes is the canonical header encoding the validator signs
// (excludes Sig and ValPubKey).
func (h *Header) SigningBytes() []byte {
	var b bytes.Buffer
	putU64(&b, h.Height)
	b.Write(h.PrevHash[:])
	b.Write(h.MerkleRoot[:])
	putU64(&b, uint64(h.Timestamp))
	putString(&b, string(h.Validator))
	putU64(&b, uint64(h.TxCount))
	b.WriteByte(h.Spec.K)
	b.WriteByte(h.Spec.M)
	b.Write(h.BodyHash[:])
	b.Write(h.LogsRoot[:])
	return b.Bytes()
}

// ID returns the block id = SHA-256 of the header signing bytes.
func (h *Header) ID() Hash { return sha256.Sum256(h.SigningBytes()) }

// Sign signs the header with the validator key pair.
func (h *Header) Sign(kp *crypto.KeyPair) {
	h.Validator = kp.Address()
	h.ValPubKey = kp.Pub
	h.Sig = kp.Sign(h.SigningBytes())
}

// VerifySig checks the validator signature and pubkey-address binding.
func (h *Header) VerifySig() error {
	if !crypto.PubMatchesAddress(h.ValPubKey, h.Validator) {
		return ErrPubAddrMismatch
	}
	if !crypto.Verify(h.ValPubKey, h.SigningBytes(), h.Sig) {
		return ErrBadSignature
	}
	return nil
}

// MerkleRootOf computes the Merkle root over the tx hashes.
func MerkleRootOf(txs []Transaction) Hash {
	items := make([][]byte, len(txs))
	for i := range txs {
		hh := txs[i].Hash()
		items[i] = hh[:]
	}
	return merkle.Root(items)
}
