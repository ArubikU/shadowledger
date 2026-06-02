# Announcement drafts (for the maintainer to post)

These are ready-to-paste drafts. **Post them yourself** — automated promotional posting to forums
violates most sites' ToS and reads as spam, so the assistant prepares copy but does not post on
your behalf. Pick the venues you like (the goal excluded Twitter/Discord/LinkedIn).

Repo: https://github.com/ArubikU/shadowledger · `go install github.com/ArubikU/shadowledger/cmd/slnode@latest`

---

## Hacker News — "Show HN"

**Title:** Show HN: ShadowLedger – a blockchain where no node stores the full chain (Go)

**Body:**
I built a blockchain on a different storage premise: instead of every node keeping the whole
history (Bitcoin full node ≈ 745 GB), each block's body is split with Reed-Solomon erasure coding
into shards, scattered deterministically across nodes by rendezvous (HRW) hashing. Any K of K+M
shards rebuild a block, so the network collectively retains history while each node stores a small
fraction plus the live account state.

Other decisions:
- No Proof-of-Work. Block production is authority-based now, designed to grow into Proof-of-Storage
  (you'd earn block rights by provably storing assigned shards — useful work, not burned watts).
- $SHARD keeps Bitcoin's monetary model (21M cap, 50-coin subsidy halving every 210k blocks) but
  pays the validator, not a miner.
- The network self-tunes shard count K/M and replication from the live node count — not config.
- Decentralized discovery (bootstrap seeds + peer-exchange gossip + optional LAN multicast), and
  late-joiner fast sync from a verified header chain + state snapshot.

It's early (v0.3, single-authority consensus, plaintext shards, no smart-contract VM yet — roadmap
in docs/GAPS-AND-DESIGN.md). I'd love feedback on the erasure-history + rendezvous-placement model
and the Proof-of-Storage direction.

Repo: https://github.com/ArubikU/shadowledger

---

## Reddit (r/golang angle)

**Title:** ShadowLedger: a from-scratch blockchain in Go where nodes store erasure-coded shards of
history instead of the full chain

**Body:**
Weekend-scale project that turned into something I'm happy with. Pure Go (stdlib + klauspost
reedsolomon). Highlights for a Go audience: deterministic canonical encoding, rendezvous hashing
for shard placement, a tiny Bloom filter, scrypt+AES-GCM encrypted wallets, net/http control +
shard channels, fast-sync. ~3.5k LOC, tests green. Honest roadmap and gap analysis in the docs.
Critique of the architecture (and the Go) very welcome. https://github.com/ArubikU/shadowledger

---

## Reddit (r/CryptoTechnology angle)

**Title:** Erasure-coded block history + rendezvous placement instead of full replication — feedback
on the trust/durability tradeoffs?

**Body:**
The idea: don't prune and don't fully replicate — erasure-code each block (K data / M parity),
place shards by HRW hashing, reconstruct on demand from any K. Tampered shards fail their committed
hash and RS decode, so retrieval is content-agnostic. No PoW; capped+halving supply paid to the
block producer. I'm specifically unsure about: (1) durability under churn without active
re-replication, (2) moving from authority to Proof-of-Storage, (3) trustless state sync (currently
header-chain-verified + trusted snapshot; planning a StateRoot commitment). Design doc:
https://github.com/ArubikU/shadowledger/blob/main/docs/GAPS-AND-DESIGN.md

---

## Lobste.rs / lemmy / forum one-liner

ShadowLedger — a Go blockchain where no node holds the full chain: blocks are erasure-coded into
shards spread by rendezvous hashing, any K-of-(K+M) rebuild them. No PoW, capped+halving $SHARD.
Early, looking for design feedback: https://github.com/ArubikU/shadowledger
