# Changelog

## v0.6.0 — Proof-of-Storage, DNS seeds, internet-only

- **Proof-of-Storage** (`internal/pos`): holders answer random shard challenges
  `H(nonce ‖ shardBytes)` over `GET /prove/{block}/{index}/{nonce}`. A per-node scoreboard tracks
  pass/miss with an `Eligible(minRatio)` gate; periodic auditor challenges shard co-holders.
  `slctl storage` / `GET /storage/scores`. See [docs/PROOF-OF-STORAGE.md](docs/PROOF-OF-STORAGE.md).
- **DNS seed discovery** — `dns_seeds:` hostnames whose A/AAAA records list live node IPs
  (Bitcoin-style bootstrap), resolved each discovery round.
- **Removed LAN multicast** — ShadowLedger targets the cloud/open internet, not local-only nets.
  Discovery is DNS seeds + explicit seeds + peer-exchange gossip.

## v0.5.0 — contract-to-contract calls

- **`CALL` opcode** — a contract can invoke another contract: push target id (callee `AddrDigest`),
  an arg word, and a gas limit. Gas is forwarded from the remaining budget, depth-limited to 8
  (reentrancy/recursion bound), no value transfer yet. Returns the callee's value or 0 on
  missing/reverted target.
- **`SELF` opcode** — push the executing contract's id.
- Nested contract storage commits are covered by the block's atomic rollback snapshot.
- New test: contract A calls contract B (counter), B's storage increments per A-call.

## v0.4.0 — smart contracts

- **Deterministic smart-contract VM** (`internal/vm`): uint64 stack machine, per-contract
  key/value storage, gas metering, revert semantics. Opcode set covers arithmetic, comparison,
  storage, control flow, caller/value/balance, calldata. See [docs/SMART-CONTRACTS.md](docs/SMART-CONTRACTS.md).
- **Contract accounts** with `Code` + `Storage`; deterministic `ContractAddress(deployer, nonce)`.
- **New tx kinds**: deploy (`Data` = bytecode) and call (`To` = contract, `Data` = input). On VM
  revert the value is refunded and only the fee is consumed; block state changes stay atomic.
- **CLI**: `slctl deploy --code prog.hex`, `slctl call --to <contract> [--data HEX] [--amount N]`.
- Note: tx wire format gained `kind`/`gas`/`data`, so v0.4 is a fresh chain (not v0.3-compatible).

## v0.3.0 — first public release

- **Encrypted `.tok` wallets** — Ed25519 private key sealed with AES-256-GCM under a
  scrypt-derived key. `slctl keygen --out w.tok --pass …` (or `SL_WALLET_PASS`).
- **$SHARD monetary policy** — 21,000,000 cap, 50-SHARD subsidy halving every 210,000 blocks,
  paid to the block validator (no mining). Fees recycled, not minted. `GET /supply`, `slctl supply`.
- **Self-tuning network** — erasure shape (K/M) and shard replication derived from the live node
  count (`internal/netparams`), not config. Producer stamps the shape per block header.
- **Decentralized discovery** — bootstrap seeds + `/hello` peer-exchange gossip + optional LAN
  multicast. No central server.
- **Late-joiner fast sync** — verified header chain + account-state snapshot (`/headers`,
  `/state/snapshot`, `chain.SyncInstall`).
- **Internet-ready** — binds all interfaces, `advertise` for public reachability, server timeouts,
  request body cap.
- Go module published as `github.com/ArubikU/shadowledger`.

## v0.2.0 (internal)

- Erasure-coded fragmented history, rendezvous shard placement, bloom index, account state,
  authority consensus, HTTP control + shard channels, on-demand reconstruction.

## Roadmap

See [docs/GAPS-AND-DESIGN.md](docs/GAPS-AND-DESIGN.md): StateRoot in header (trustless sync),
active shard re-replication on churn, Proof-of-Storage challenges, multi-validator/fork-choice,
smart-contract VM, protobuf/libp2p wire.
