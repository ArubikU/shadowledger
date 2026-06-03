# Consensus & minting — how it works, and the path to BTC/ETH-grade security

Direct answers to common questions, then the design and the honest gap.

## Who mints? (v0.23: permissionless multi-validator)

Mainnet now runs **`postorage`**: any node can become a validator by locking a bond + solving the
registration PoW (`slctl register`), entering the on-chain registry, and getting elected in the
per-(height,prevHash,round) rotation. Competing branches are resolved by the **reorg engine**
(heaviest chain wins; state rewinds + replays), so the network converges without coordination. The
founder is just the genesis/bootstrap validator — not the only one.

Earlier (v0–v0.22) mainnet was single-authority (founder signs every block). That path still exists
(`consensus: authority`) for private/federated deployments.

## Is anything final? (v0.24: depth-based finality)

Yes. A block `FinalityDepth` deep (16 on mainnet) is **irreversible** — no reorg can rewind below it.
The chain keeps a *replay base* (the rewind floor for reorgs); as the head advances, `advanceFinality`
moves that floor up to `head − FinalityDepth` and persists it, never moving back. A reorg rewinds only
down to the replay base, so a competing branch that forks *below* the finalized height literally can't
be applied (it can't reach the base to replay from) and is refused. This bounds how far history can ever
change to the last `FinalityDepth` blocks: ~80s at a 5s block time. `Finalized()` reports the height.

## Is there Proof-of-Work?

**Yes, but only as a Sybil gate — never to mine blocks.** A node must solve a one-time PoW puzzle to
*register* as a validator (`internal/regpow`, v0.20), on top of its bond. This costs real compute so
you can't spin up thousands of fake validators, and the puzzle hash seeds shard-assignment
randomness — exactly Zilliqa/QuarkChain's model. Block *production* burns no energy: minting rights
come from **useful work — proving you store the network's data**, not from hashing.

How people compete:
- **To enter:** spend compute (registration PoW) + lock a bond (PoS).
- **To earn (mint):** store more assigned shards and prove it — proven storage weights leader
  election (the ongoing race; deterministic weighting lands with on-chain storage proofs).

## How does a follower with no coins get started? (v0.26 PoW faucet)

Validators mint via the coinbase, but a brand-new follower has zero $SHARD — can't pay gas, can't
post a bond. The **PoW faucet** is the on-ramp: mine a nonce so `H(chainID ‖ yourAddr ‖
recentBlockBodyHash ‖ nonce)` has N leading zero bits, submit a `KindFaucet` tx (fee 0), and receive
a small fixed payout from the treasury. `slctl faucet --wallet w.tok`. It is Bitcoin-style work, but
the target is anchored to a recent block's committed `BodyHash` (the commitment to that block's
shards) — so every node verifies it from the signed header alone, no full body needed. Payout is from
the treasury (not newly minted → cap untouched); a per-address cooldown + the hashing cost rate-limit
it. Enough to bootstrap deploying a contract or saving toward a validator bond.

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
6. ✅ Fork choice (v0.21, `internal/forkchoice`): deterministic heaviest-chain tip selection.
7. ✅ Reorg engine (v0.22) + wired live (v0.23): state rewind to the replay base + replay of the
   heaviest branch; a partition resolves automatically once the heavier branch is seen.
8. ✅ Finality (v0.24): blocks `FinalityDepth` deep are irreversible — the replay base (reorg floor)
   advances to `head − FinalityDepth` and never moves back, so no reorg can rewind finalized history.
   **Remaining:** storage-weighted fork choice (today weight = chain length; deterministic
   storage-proof weighting needs on-chain proof records) + external audit.

## Fork choice by availability (the design, v0.19 building block)

The intended fork-choice fits ShadowLedger's thesis: the canonical chain is the one whose blocks are
**data-available** — i.e. reconstructable from the pooled fragments. Mechanism:

- A node never holds all of a block's shards. Holders pool their fragments (served on the shard
  channel); any node that gathers `K` of `K+M` reconstructs the body and checks it against the
  validator's **signed commitment** (`BodyHash` + `MerkleRoot`). That is `chain.VerifyAvailable`
  (v0.19) / `GET /verify/{height}` — *reconstruct from pooled fragments, compare to the committed
  hash*. (Note: Reed-Solomon is all-or-nothing — fewer than `K` shards recover nothing, so there is
  no "partial reconstruction"; the pool must collectively hold `K`.)
- A block that cannot be reconstructed to its commitment is **withheld/unavailable** and is not
  trusted. Fork choice then prefers the chain with more availability-verified (and storage-proven)
  blocks — "compare the reconstructions, pick the available one." The full reorg engine
  (rewind to common ancestor, replay the heavier-available branch) is the remaining work; v0.19
  ships the per-block availability proof it builds on.

### Selection rule (v0.21, `internal/forkchoice`)

The deterministic rule is implemented + tested: a block tree whose canonical tip is the one with the
**heaviest cumulative weight** (ties → lowest block id), with `CommonAncestor` for the rewind point.
Per-block weight is pluggable, meant to be storage/availability weight (heaviest = most-available).
Every node computes the identical tip → convergence without coordination. **Reorg engine (v0.22):** `chain.AcceptBlock` now does it — accepts blocks on any known branch into
the tree and, when a branch becomes heavier, **rewinds state and replays the winning branch** onto a
fresh copy of the genesis state, committing only if it applies cleanly. Tested (two producers fork →
node reorgs to the heavier branch with its exact state). **Final integration left:** route live
postorage gossip through `AcceptBlock`, persist side-branch bodies, and give fast-synced nodes a
genesis (or checkpoint) state to rewind to. The algorithm is done; mainnet stays on the linear
authority path until that wiring + the switch to postorage.
6. On-chain storage-proof records → enforceable eligibility + reward/fee share + slashing of failed
   proofs (storage-failure slashing; equivocation already done)
7. Finality gadget, hardened P2P, audit, public testnet
