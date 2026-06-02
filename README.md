# ShadowLedger ($SHARD)

Blockchain where **no node stores the full chain**. Each block's history is split with
Reed–Solomon erasure coding into shards, scattered deterministically across nodes via
rendezvous hashing. Any `k` of `m` shards rebuild a block. New nodes sync by holding their
assigned fragments + the live account state — not gigabytes of history.

No Proof-of-Work. Block production is authority/validator-driven (v0), with a consensus
interface designed to grow into Proof-of-Storage.

**Monetary policy ($SHARD):** Bitcoin-style — 21,000,000 cap, 50-SHARD block subsidy halving
every 210,000 blocks, paid to the block's validator (no miners, no burned energy). Fees are
recycled, not minted.

**Self-tuning:** the operator does NOT configure shard counts. The network derives the erasure
shape (K data / M parity) and replication from the live node count (`internal/netparams`); the
producer stamps the chosen shape into each block header.

**Decentralized, no central server:** nodes find each other via bootstrap **seeds** (any running
node), transitive **peer-exchange** gossip, and optional **LAN multicast**. A fresh node
**fast-syncs** a verified header chain + state snapshot, then participates.

**Smart contracts:** a small deterministic stack VM with gas + per-contract storage. Deploy and
call on-chain code (`slctl deploy` / `slctl call`). See [docs/SMART-CONTRACTS.md](docs/SMART-CONTRACTS.md).

## Why

- Full Bitcoin node today: ~745 GB and growing. Barrier to running a node.
- ShadowLedger: a node holds `~ (k/m) * chain_size / N` plus the small active state.
- Lose up to `m-k` shard-holders per block and the block still reconstructs.
- Tampered shards fail Merkle inclusion + RS decode → auto-rejected, content-agnostic.

## Layout

```
cmd/slnode   node daemon (control :4004, shard transfer :4005)
cmd/slctl    wallet + cli (keygen, balance, send, inspect)
internal/
  crypto     Ed25519 keys, addresses, signatures
  merkle     Merkle tree + inclusion proofs
  types      Transaction, Block, Header
  erasure    Reed-Solomon encode/decode (klauspost) + shard hashing
  rendezvous deterministic shard -> node assignment
  bloom      local "which shard do I hold" filter
  economy    $SHARD supply cap + halving emission schedule
  netparams  network-decided adaptive K/M + replication
  state      account ledger (balances + nonces + minted supply)
  store      on-disk blocks (headers) + shards
  chain      validation, append, reconstruction, fast-sync
  consensus  validator interface + v0 authority impl
  mempool    pending validated txs
  p2p        HTTP control + shard channels, discovery, sync
  node       wiring
docs/        SPEC-CORE-V1.md, ARCHITECTURE.md, GAPS-AND-DESIGN.md
```

## Install

```
go install github.com/ArubikU/shadowledger/cmd/slnode@latest
go install github.com/ArubikU/shadowledger/cmd/slctl@latest
```

Or from source: `go build ./...` (binaries `slnode`, `slctl`).

## Quick start

```
go test ./...

# generate an ENCRYPTED wallet (.tok). Passphrase via --pass or SL_WALLET_PASS.
slctl keygen --out wallet.tok --pass "your strong passphrase"
# (plaintext .json wallets also work for dev: slctl keygen --out wallet.json)

# run a validator node (genesis funds the configured address)
slnode --config node.yaml

# check balance / send
slctl balance --addr <addr> --rpc http://localhost:4004
slctl send --wallet wallet.tok --pass "…" --to <addr> --amount 100 --rpc http://localhost:4004
slctl supply --rpc http://localhost:4004     # $SHARD minted + next reward
slctl peers  --rpc http://localhost:4004     # discovered peers
```

## Running on the public internet (not just LAN)

A node binds all interfaces (`:4004` control, `:4005` shard). To be reachable by others:

1. Set `advertise:` in config to your public host/IP (and forward both ports on your router/cloud
   firewall, or run on a VPS with a public IP). This is the same requirement as a Bitcoin full node.
2. New nodes join by listing any reachable node under `seeds:` — peer-exchange spreads the rest.
   On the same LAN, `lan_discovery: true` finds peers with zero config.

> Honest caveat: there is no automatic NAT hole-punching yet (libp2p-style) — you forward a port.
> See [docs/GAPS-AND-DESIGN.md](docs/GAPS-AND-DESIGN.md) §3–6.

## Docs

[SPEC-CORE-V1.md](docs/SPEC-CORE-V1.md) · [ARCHITECTURE.md](docs/ARCHITECTURE.md) ·
[SMART-CONTRACTS.md](docs/SMART-CONTRACTS.md) ·
[GAPS-AND-DESIGN.md](docs/GAPS-AND-DESIGN.md) (PoW decision, discovery, onboarding, roadmap) ·
[CHANGELOG.md](CHANGELOG.md)
