# ShadowLedger — security status & threat model

Honest inventory of what protects the chain, what doesn't yet, and the priority order.
This is a young project (v0.x) — **do not put real value on mainnet**. Treat it as a testnet.

## What is in place

| Area | Mechanism |
|---|---|
| Tx authenticity | Ed25519 signatures; pubkey must hash to `From`; tampering breaks the sig |
| Replay (same chain) | per-account monotonic **nonce** |
| Replay (cross network) | **ChainID** bound into the signed tx (v0.16) — a tx from net A is invalid on net B |
| Block authenticity | header signed by an authorized validator; `AuthorizeHeader` checks sig + set |
| History integrity | erasure shards committed by hash in `ShardSet`; bad shard fails decode + is dropped |
| Log integrity | merkle `LogsRoot` in the signed header; reconstructed logs verified against it |
| Sybil cost (validators) | on-chain **bond** (`MinBond`) to register |
| Misbehavior (validators) | **equivocation slashing** — double-signing burns the bond |
| Storage liveness | Proof-of-Storage challenges + scoreboard (advisory gate) |
| DoS hygiene | local **banlist** (strikes → temp ban), request body cap, HTTP timeouts |
| Funds at rest | **.tok** wallets (scrypt + AES-256-GCM) |
| Supply | fixed cap + halving; coinbase validated against the emission schedule |
| State transitions | atomic block apply with full rollback on any error |

## What is NOT in place (gaps, by priority)

1. **Consensus liveness / fork choice** — PoStorage elects one leader per height with no
   timeout; an offline elected validator stalls the chain, and there is no rule to choose between
   competing chains. **Mainnet runs single-authority** until this lands. *(highest priority)*
2. **Enforceable storage proofs** — proofs are posted/scored but eligibility + slashing are not yet
   driven by *on-chain* proof records, so the storage gate is advisory, not enforced.
3. **Trustless state sync** — fast-sync verifies the header chain but trusts the peer's state
   snapshot (assumeutxo-style). Needs a `StateRoot` commitment in the header.
4. **Contract safety** — VM arithmetic is wrapping `uint64` (no SafeMath); a token contract can
   overflow. No reentrancy issues today (no value-bearing cross-contract calls), but no formal
   gas-price market either. Audit contracts before trusting them.
5. **Transport security** — P2P is plain HTTP. No TLS, no peer authentication of endpoints, no
   eclipse-attack resistance, no authenticated gossip. Run behind a firewall; assume the network is
   observable.
6. **Light-client proofs** — roots exist (`MerkleRoot`, `LogsRoot`) but inclusion-proof endpoints
   are not exposed, so light clients must trust a full node.
7. **Key/ops** — the founder/validator key sits on the VPS to sign blocks; compromise = chain
   authority compromise. The auto-updater trusts GitHub releases (checksum-verified) — lock down
   that account (2FA).
8. **No formal audit, no fuzzing of the VM/encoders, no testnet at scale.**

## Threat notes

- **Malicious block injection: blocked, and proven.** `ApplyExternalBlock` rejects forged blocks at
  every field — bad/flipped signature, unauthorized validator, tampered body (merkle), wrong
  height/prev, future timestamp, forged logs root. Covered by `TestRejectMaliciousBlocks`
  (7 attack variants, all rejected). An outsider cannot insert a block.
- **Forged history:** a validator cannot rewrite past blocks (prev-hash chain + signatures), nor
  forge events (LogsRoot), nor mint outside the schedule (coinbase check). It *can* censor or stall
  (single authority today) — addressed by gaps #1/#2. A bonded attacker building a competing VALID
  chain is the fork-choice/finality gap (#1).
- **Lying shard server:** detected by the committed shard hash; the liar is skipped and struck
  (banlist).
- **Cross-network replay:** blocked by ChainID. **Same-network replay:** blocked by nonce.
- **Sybil validators:** cost a bond; double-signers are slashed. But without #1/#2 a permissionless
  multi-validator mainnet is not safe yet.

## Roadmap (security-ordered)

1. Leader timeout + fork choice (unblocks safe multi-validator) →
2. On-chain storage-proof records + slashing for missed proofs →
3. `StateRoot` in header (trustless sync + light proofs) →
4. SafeMath/checked arithmetic in the VM, gas-price market →
5. Authenticated/encrypted transport, peer discovery hardening →
6. External audit + public testnet before any 1.0.
