package types

import (
	"bytes"
	"encoding/binary"

	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/merkle"
)

// Log is an event emitted by a contract via the LOG opcode. Logs are a hybrid:
//   - Ethereum-style: a merkle LogsRoot over the block's logs is committed in the
//     block header (signed, part of the block id) — tamper-evident, verifiable.
//   - ShadowLedger-style: the log DATA itself is erasure-coded and rendezvous-
//     distributed like block bodies — no node stores the whole log history; it is
//     reconstructed on demand and checked against the committed LogsRoot.
type Log struct {
	Contract crypto.Address `json:"contract"`
	Topics   []uint64       `json:"topics"`
	TxIndex  int            `json:"tx_index"`
}

// canonical encodes a log deterministically for the merkle leaf.
func (l *Log) canonical() []byte {
	var b bytes.Buffer
	putString(&b, string(l.Contract))
	putU64(&b, uint64(l.TxIndex))
	putU64(&b, uint64(len(l.Topics)))
	for _, t := range l.Topics {
		putU64(&b, t)
	}
	return b.Bytes()
}

// LogsRootOf is the merkle root over a block's logs (zero hash if none). This is
// the Ethereum-style commitment placed in the header.
func LogsRootOf(logs []Log) Hash {
	if len(logs) == 0 {
		return merkle.Zero
	}
	items := make([][]byte, len(logs))
	for i := range logs {
		items[i] = logs[i].canonical()
	}
	return merkle.Root(items)
}

// EncodeLogs / DecodeLogs are the canonical wire form of a block's logs (the
// bytes that get erasure-coded into log-shards).
func EncodeLogs(logs []Log) []byte {
	var b bytes.Buffer
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(logs)))
	b.Write(n[:])
	for i := range logs {
		putBytes(&b, logs[i].canonicalFull())
	}
	return b.Bytes()
}

// canonicalFull is canonical() — logs carry no signature, so it is the full form.
func (l *Log) canonicalFull() []byte { return l.canonical() }

// DecodeLogs reverses EncodeLogs.
func DecodeLogs(data []byte) ([]Log, error) {
	if len(data) < 4 {
		return nil, ErrBadBody
	}
	n := binary.BigEndian.Uint32(data[:4])
	r := bytes.NewReader(data[4:])
	out := make([]Log, 0, n)
	for i := uint32(0); i < n; i++ {
		raw, err := readBytes(r)
		if err != nil {
			return nil, err
		}
		l, err := decodeLog(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

func decodeLog(data []byte) (Log, error) {
	r := bytes.NewReader(data)
	var l Log
	contract, err := readBytes(r)
	if err != nil {
		return l, err
	}
	l.Contract = crypto.Address(contract)
	idx, err := readU64(r)
	if err != nil {
		return l, err
	}
	l.TxIndex = int(idx)
	n, err := readU64(r)
	if err != nil {
		return l, err
	}
	l.Topics = make([]uint64, n)
	for i := uint64(0); i < n; i++ {
		if l.Topics[i], err = readU64(r); err != nil {
			return l, err
		}
	}
	return l, nil
}
