# Consensus & minting — how it works, and the path to BTC/ETH-grade security

Direct answers to common questions, then the design and the honest gap.

## Who mints today? (v0.7)

**One node: the founder validator.** ShadowLedger v0–v0.7 ships a *single-authority* engine — the
founder key signs every block; others verify the signature and apply. This is centralized: secure
against outsiders forging blocks (they lack the key), but you must trust the founder not to censor.
It is **not** Bitcoin/Ethereum-grade decentralization yet. Stated plainly so nobody is misled.

## Is there Proof-of-Work?

**No.** By design — the project's whole thesis is "no burned electricity." Minting rights come from
**useful work: proving you store the network's data**, not from hashing.

## Does everyone mint?

Not yet. The goal is: **any node that proves it stores its assigned shards can be in the minting
rotation.** That's the upgrade below.

## The model your question describes (and it's the right one)

> "the machine resembles [reconstructs] the files... net validates that the shards after
> reconstruction are valid → approve"

That is **Proof-of-Storage / Proof-of-Retrievability as consensus**:

1. **Storage is the scarce resource.** Rendezvous hashing assigns each node specific shards. To be a
   validator you must actually hold them.
2. **Random challenges prove possession.** `H(nonce ‖ shardBytes)` with an unpredictable nonce — a
   node can't fake it without the bytes (already implemented: `internal/pos`, `/prove`).
3. **Reconstruction validates content.** A block's body is only accepted if its `K`-of-`K+M` shards
   reconstruct and every shard matches its committed `ShardHash` (already enforced in `chain` +
   `erasure`). Forged/garbage data fails the math and is rejected — content-agnostic, no human
   review. (Your "70% real content" intuition = you must serve enough valid shards to pass; a node
   that can't reconstruct its assignment is not eligible.)
4. **Eligibility → minting.** Nodes passing storage proofs join the validator set; one is elected
   per block.

## v0.8: PoStorage leader election (this release)

`internal/consensus` now has a second engine, **`PoStorage`**, alongside `Authority`:

- **Deterministic leader per height.** `LeaderFor(height, prevHash)` = the validator maximizing
  `SHA-256(prevHash ‖ addr ‖ height)` (HRW). Every node computes the same leader from on-chain data
  (the validator set + previous block hash) — no coordination, exactly one leader per height, so no
  forks from disagreement.
- **Rotation.** Across heights the winner changes, so minting rotates among validators instead of a
  single authority. With N validators each mints ~1/N of blocks.
- **`AuthorizeHeader`** accepts a block only if it is signed by *that height's elected leader*.
- **Storage gate (advisory now).** A node won't try to lead if its own storage score is below
  threshold (`pos.Scoreboard.Eligible`). 

Select it with `consensus: postorage` in config (default stays `authority` while mainnet has one
node — with a single validator both engines behave identically and safely).

## The honest gap to Bitcoin/Ethereum security

What v0.8 gives: **permissioned multi-validator** (PoA/Clique-grade) — decentralized *among a known
validator set*, deterministic, fork-free. Good for a federation.

What full Nakamoto/Ethereum security additionally needs (NOT done — do not claim it):

1. **Permissionless validator entry.** Anyone can join by registering on-chain and posting a
   **storage bond/stake**. Needs a registration tx + on-chain validator state.
2. **On-chain proof records.** Storage pass/miss must be recorded *on-chain* (a proof tx), so
   eligibility is computed identically by all from chain state — not from each node's local
   scoreboard (local scores diverge → forks). This is the crucial step that makes the storage gate
   *enforceable* rather than advisory.
3. **Slashing.** Failed proofs / equivocation burn the bond — the economic stick that deters sybils
   and liars. Without cost-to-misbehave there is no Nakamoto security.
4. **Fork choice + finality.** A rule to pick among competing chains (heaviest-stake / GHOST-like)
   and a finality gadget, plus equivocation detection.
5. **Hardened P2P.** Authenticated transport, DoS limits, eclipse-attack resistance, NAT traversal.
6. **Audits + testnet + economic review.** Mandatory before real value rides on it.

That is months of work and external review. v0.8 is a real, tested step (decentralized minting among
validators); it is not, and is not advertised as, equal to mainnet Bitcoin/Ethereum security.

## v0.9: on-chain validator registry + bond (permissionless entry)

Validators are now **on-chain state**, not config. Two transaction kinds:

- `register` (`KindRegister`): locks `Amount` as a **bond** (≥ `economy.MinBond` = 1,000 SHARD) and
  adds you to `state.Validators`. `slctl register --wallet w.tok --bond <baseunits>`.
- `unregister` (`KindUnregister`): removes you and returns the bond.

`PoStorage` reads the live set from `state.ActiveValidators()` — so **anyone who posts a bond joins
the minting rotation**, with no config change, and every node computes the same set from identical
chain state (deterministic → no forks over membership). Genesis seeds the registry with the founder.
Query it: `slctl validators`.

This is the **permissionless-entry** mechanism. Two things still gate turning it on for mainnet:

- **Liveness:** if an elected leader is offline, that height has no block and the chain stalls —
  there is no leader-timeout fallback / fork-choice yet. (Verified in testing.) Mainnet therefore
  still runs `authority` until fork-choice lands.
- **Enforceable storage:** the bond is locked but not yet *slashed* for failed storage proofs — that
  needs on-chain proof records (next).

## Roadmap order

1. ✅ Storage challenges + scoreboard (v0.6)
2. ✅ PoStorage leader election among validators (v0.8)
3. ✅ On-chain validator registry + storage bond — register/exit txs (v0.9)
4. ✅ Equivocation slashing — KindSlash burns the bond of a validator that double-signs, 10% bounty
   to the reporter, permanent bar (v0.10). Local peer banlist for DoS hygiene (v0.10).
5. ✅ Liveness: **leader-timeout / round fallback** (v0.17). The leader for a height is
   `HRW(prevHash, height, round)`; round = elapsed-since-head / blockTime. If round-0's leader is
   offline for one block-time, round 1 elects a different validator who may produce. Headers carry
   `Round`; `ApplyExternalBlock` validates round-timing (`ts >= prevTs + round*blockTime`, not far
   future) so a high round can't be claimed early. Genesis uses a fixed timestamp to anchor timing.
   **Remaining:** true reorg-based fork choice — today it's first-valid-block-per-height-wins (no
   reorg), so a network partition could leave nodes on different tips until manual intervention.
   Heaviest-chain reorg + finality is the next consensus item.
6. On-chain storage-proof records → enforceable eligibility + reward/fee share + slashing of failed
   proofs (storage-failure slashing; equivocation already done)
7. Finality gadget, hardened P2P, audit, public testnet
