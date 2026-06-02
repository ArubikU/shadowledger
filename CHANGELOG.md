# Changelog

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
