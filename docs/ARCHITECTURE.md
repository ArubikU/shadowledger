# ShadowLedger Architecture

## Data flow: producing a block

```
mempool txs ──► validator orders + builds body (canonical tx bytes)
            ──► Merkle root over tx hashes
            ──► RS encode body into K+M shards  (internal/erasure)
            ──► ShardSet{blockID,K,M,shardHashes} committed in Header
            ──► header signed (validator ed25519)
            ──► header+ShardSet gossiped to all peers (cheap, :4004)
            ──► each shard pushed to its top-R rendezvous holders (:4005)
            ──► every node applies block to active state (balances/nonces)
```

The heavy block body never travels whole to everyone — only the small header+ShardSet
broadcasts; shard bytes go only to the nodes that must hold them.

## Data flow: reconstructing history (slow path)

```
need body of block b ──► read ShardSet from local header
                     ──► rank holders per shard (rendezvous)
                     ──► parallel GET :4005/shard/b/i from top holders
                     ──► verify shard bytes vs ShardSet hashes (drop liars)
                     ──► once K valid shards: RS-decode ──► body ──► txs
```

Corruption is content-agnostic: a forged shard either fails its `ShardHash` or breaks RS
decode; it is dropped and refetched from another holder.

## Storage math

`N` nodes, replication `R`, params `(K,M)`. Each block body of size `B`:
- total shard bytes on the network ≈ `B * (K+M)/K * R`.
- per node ≈ `B * (K+M)/K * R / N`.
As `N` grows, per-node history storage → small, while the active state stays full on every
node. That is the whole point: validation needs the state, not the raw history.

## Consensus evolution

- **v0 authority**: one configured validator key produces blocks; peers verify the header
  signature against the validator set. No forks assumed (single producer).
- **[future] Proof-of-Storage**: the rendezvous assignment already binds each node to specific
  shards. A challenge protocol (`prove you hold shard (b,i)` via a random byte-range + Merkle
  path over the shard) lets the network weight a node's block-production rights by *useful
  storage served*, replacing PoW's wasted compute. Slashing for failed proofs.
- **[future] multi-validator BFT**: rotate producers; fork-choice by validator-set signatures.

## Module dependency graph

```
crypto ◄── types ◄── state
   ▲         ▲   ◄── chain ──► erasure, merkle, store, rendezvous, bloom
   │         │           ▲
consensus ───┘           │
   ▲                     │
   └──── node ──► p2p ────┘
```

No import cycles: `types` depends only on `crypto`+`merkle`; everything heavy depends on
`types`; `node` wires it all.
