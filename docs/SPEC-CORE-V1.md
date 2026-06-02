# ShadowLedger Core Spec v1

Status: v0 implementation target. Sections marked **[future]** are interface-only.

## 1. Identity & crypto

- Signature scheme: **Ed25519** (stdlib `crypto/ed25519`).
- Address = first 20 bytes of `SHA-256(pubkey)`, hex-encoded with `sl` prefix.
  - `Address = "sl" + hex(sha256(pubkey)[:20])`
- Wallet file (JSON): `{ "priv": base64, "pub": base64, "address": "sl..." }`.

## 2. Transaction

Account model (not UTXO) for v0.

```
Transaction {
  From    Address   // sender, derived from PubKey
  To      Address
  Amount  uint64    // base units of SHARD
  Fee     uint64
  Nonce   uint64    // per-account, strictly increasing from 0
  PubKey  []byte    // sender ed25519 pubkey (must hash to From)
  Sig     []byte    // ed25519 over SigningBytes()
}
```

- `SigningBytes` = canonical encoding of all fields except `Sig`.
- `Hash` (TxID) = `SHA-256(canonical-encoding-including-Sig)`.
- Validity: pubkey hashes to From; sig verifies; nonce == account.Nonce; balance >= Amount+Fee.

Coinbase / genesis txs have `From = ""` and no signature; only allowed at genesis or as the
single block reward tx (v0 reward = 0, fees go to validator).

## 3. Block

```
Header {
  Height     uint64
  PrevHash   Hash      // SHA-256 of previous header
  MerkleRoot Hash      // root over tx hashes
  Timestamp  int64     // unix seconds
  Validator  Address   // who produced it
  TxCount    uint32
  ShardSpec  { K, M uint8 }   // erasure params used to fragment THIS block body
  BodyHash   Hash      // SHA-256 of canonical tx list (the erasure-coded payload)
}

Block { Header; Txs []Transaction }
```

- `Header.Hash` = `SHA-256(canonical header bytes)`. This is the block id.
- Genesis: Height 0, PrevHash = zero, Validator = genesis authority, Txs = initial funding.

## 4. Merkle tree

- Leaves = `SHA-256(0x00 || txhash)`; internal = `SHA-256(0x01 || left || right)`.
- Odd node duplicated. Empty tx set → root = zero hash.
- Inclusion proof = list of (sibling, isLeft).

## 5. Erasure coding (the ShadowLedger core)

- Block **body** (canonical tx list bytes) is the unit that gets fragmented.
- Reed-Solomon `(K data, M parity)` → `K+M` shards, any `K` reconstruct.
- v0 default `K=4, M=2` (tolerate 2 lost shards per block).
- Shard `i` payload hashed: `ShardHash_i = SHA-256(blockID || i || bytes)`.
- A block's `ShardSet` = `{ blockID, K, M, paddedLen, [ShardHash_0..K+M-1] }`, signed/committed
  inside metadata so corrupt shards are detectable before decode.

## 6. Shard placement — rendezvous (HRW) hashing

For each shard `s` of block `b`, score every node `n`:

```
score(n, b, s) = SHA-256(nodeID || blockID || shardIndex)  // as big-endian uint
```

Top `R` nodes by score are the **primary holders** (R = replication factor, v0 R=2).
Deterministic, stable under node churn, no global coordination. A node knows it must hold
shard `s` iff it ranks in the top `R` for `(b,s)`.

## 7. Local shard index — Bloom filter

Each node keeps a Bloom filter of held `ShardHash` values for O(1) "do I have it?" checks
before hitting disk, plus an exact on-disk index for truth.

## 8. State / ledger

- `Account { Balance uint64; Nonce uint64 }` keyed by Address.
- Apply block: for each tx in order, debit From (Amount+Fee), credit To, bump From.Nonce,
  credit fees to Validator. State is a deterministic function of the block sequence.
- The **active state** is what every node keeps fully; **history** is what gets sharded.

## 9. Consensus

- v0: **single authority validator** signs and produces blocks every `BlockTime` (default 5s)
  if mempool non-empty (or on demand). Other nodes verify header sig against the configured
  validator set and apply.
- `Consensus` interface: `ProposeBlock`, `ValidateHeader`, `Validator(height)`. **[future]**
  Proof-of-Storage: holders periodically prove possession of assigned shards to earn the right
  to validate; sketched in ARCHITECTURE.md.

## 10. Networking

Two ports, HTTP/JSON in v0 (protobuf/gRPC **[future]**):

- **:4004 control / RPC**
  - `POST /tx`            submit signed tx → mempool
  - `GET  /account/{addr}` balance + nonce
  - `GET  /head`          current head header
  - `GET  /block/{id}`    header + shardset (NOT full body)
  - `GET  /peers`         known peers
  - `POST /gossip/block`  receive a newly produced block header+shardset
  - `POST /gossip/tx`     receive a tx
- **:4005 shard transfer**
  - `GET  /shard/{blockID}/{index}` raw shard bytes (if held)
  - `GET  /have/{blockID}`          which shard indices this node holds

Reconstruction (slow path): to materialize a block body, fetch `K` shards in parallel from
top-ranked holders (rendezvous), verify each against `ShardSet` hashes, RS-decode.

## 11. Persistence

```
<datadir>/
  keys/node.json          node identity (ed25519)
  state.json              account snapshot at head (+ head height)
  blocks/<height>.hdr     header + shardset json
  shards/<blockID>/<i>.shard   shard bytes this node is responsible for
  bloom.dat               serialized bloom filter
```

## 12. Out of scope for v0 (tracked)

- BFT / multi-validator consensus, slashing, real Proof-of-Storage challenges.
- KZG / Verkle commitments (Merkle only in v0).
- Encrypted shards (privacy); v0 shards are plaintext erasure fragments.
- protobuf wire format, libp2p, NAT traversal, peer discovery beyond static config.
