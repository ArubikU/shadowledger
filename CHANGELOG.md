# Changelog

## v0.27.0 — storage-weighted consensus (on-chain proof-of-storage)

- **On-chain storage proofs.** New `KindStorageProof` tx: a validator proves it can produce a
  *protocol-assigned* shard of a recent confirmed block. The shard index is derived as
  `H(addr ‖ blockID) mod T` (no cherry-picking), and the bytes are verified against the committed
  shard hash from the persisted header/shard set — no full body, no header change, no reset.
- **Storage-weighted leader election.** `state.StorageWeights` gives each validator
  `1 + min(score, cap)` virtual HRW entries; `PoStorage.LeaderFor` elects over those entries
  (integer-only, deterministic). Block-production odds now track *proven* storage; a validator that
  stops proving decays to the base weight. This makes the consensus genuinely storage-coupled in the
  running binary (previously: uniform election + advisory scoreboard).
- **Validator auto-proof loop.** A postorage validator periodically derives its assigned shard,
  obtains it (local or by reconstruction), and submits a storage proof, accruing election weight.
- **Honest scope:** still *not* done — bond *slashing* for missed proofs (today: weight decay) and
  storage-weighted *fork choice* (branch weight is still chain length). Documented, not hidden.
- Chainparams: `StorageWindow=256`, `StorageMinDepth=4`, `StorageCap=64`. Tests: proof credited /
  bad-shard rejected / stale+shallow rejected / weight decays / 65:1 weighting wins >90%. Consensus
  change → validators must run v0.27+.

## v0.26.0 — PoW faucet (follower on-ramp to $SHARD)

- **Earn $SHARD by Proof-of-Work, no bond, no balance.** New `KindFaucet` tx + `internal/faucet`:
  a follower mines a nonce so `H(chainID ‖ addr ‖ anchorBodyHash ‖ nonce)` has N leading zero bits
  (Bitcoin-style), then claims a small fixed payout. Validators still mint via the coinbase; this is
  the on-ramp for everyone else.
- **Anchored to a recent block's committed `BodyHash`** — the commitment to the body that becomes
  that block's erasure shards. The anchor is in the *signed header*, so **every node verifies a claim
  without holding the (fragmented) body** — consensus-safe in a no-full-history network. It's unknown
  until the block is produced, so solutions can't be precomputed and the target rotates with the chain.
- **Paid from the treasury, not freshly minted** → the 21M cap + halving schedule are untouched.
  Per-address cooldown (50 blocks) rate-limits; anchor must be confirmed (≥4 deep) and recent (≤256).
  Sybil cost = the hashing per address.
- `slctl faucet --wallet w.tok --rpc URL` mines the PoW and submits the claim (fee 0 — a brand-new
  empty wallet can earn its first coins).
- Chainparams: `FaucetAmount` (2 SHARD), `FaucetBits` (18), `FaucetCooldown` (50), `FaucetDepth` (4).
  State persists per-address claim heights; replay/finality clones inherit faucet+PoW config
  (`State.InheritConfig`) so reorg/finality replay validates faucet txs identically.
- Tests: PoW solve/verify (anchor/addr/chain-bound), claim pays from treasury, bad-PoW rejected,
  cooldown enforced, unconfirmed-anchor rejected. Consensus note: `KindFaucet` changes block
  execution → all validators must run v0.26+ (mainnet validator upgraded).

## v0.25.0 — SHL goes Solidity-like (contracts rewritten)

- **Functions + selector dispatch.** `fn name(params){…}` with an auto-generated dispatcher: calldata
  word 0 is the selector `be8(sha256(name))` (the analogue of Solidity's 4-byte keccak selector),
  params bind from words 1.. , and an unknown selector **reverts**. No more hand-rolled
  `if (arg(0)==1)` chains.
- **Named state + mappings — no magic numbers.** `state x;` gets a compiler-assigned slot; use it as
  a scalar or a mapping `x[k]` (storage key `MIX(slot,k)`, a new keccak-equivalent VM opcode, so
  mappings never collide). Replaces the old `store[18446744073709551614]` reserved-key hacks.
- **`require` / `revert`.** Real revert (new `REVERT` opcode): a failed check discards *all* storage
  writes and charges gas — not the old silent `return 0;` no-op.
- **Overflow-checked arithmetic by default** (Solidity 0.8 semantics): new `ADDC/SUBC/MULC` opcodes
  revert on overflow/underflow; `/ %` already revert on zero. `+ - *` in SHL emit the checked forms.
- **`msg.sender` / `msg.value`** aliases.
- **Contracts rewritten** in the new dialect: `token.shl`, `nft.shl`, `memecoin.shl` (counter stays
  flat to demo backward-compat). Flat scripts still compile unchanged.
- **`slctl call/query --fn NAME --args a,b`** computes the selector + packs the calldata for you.
- Tests: selector dispatch, mapping reads/writes, require-reverts-and-undoes, unknown-selector
  revert, checked overflow/underflow, all repo contracts compile. Honest scope in docs/SHL.md: this
  is a faithful Solidity *subset* (uint64, no 256-bit/structs/inheritance/ABI), not a drop-in.

## v0.24.0 — finality (hard irreversibility)

- **Depth-based finality:** blocks `FinalityDepth` behind the head (16 on mainnet) become
  **irreversible**. The chain tracks a *replay base* (the reorg rewind floor); `advanceFinality`
  walks head→base, replays every block at height ≤ `head-F` onto a base clone, and advances the
  base + persists it. Once advanced, the floor never moves back.
- **Reorg can't rewind below the finalized height.** A divergent branch reorg walks down to the
  replay base (`walkDown(best, replayBaseID)`); a fork that diverges *below* the finalized floor
  can't reach the base → `ok=false` → the reorg is refused and the head is kept. Deep history is
  locked. So a heavier-but-ancient branch can no longer erase finalized blocks.
- Wired into all three head-advancing paths: `ProduceBlock`, `ApplyExternalBlock`, and both the
  fast-forward and divergent branches of `reorgToBest`.
- `Finalized()` exposes the finalized height; replay base (id/height/state) persisted + reloaded so
  finality survives restarts. Tested: 6 blocks @ F=3 → finalized height 3, reorg floor at 3.
- `FinalityDepth` is a chainparam (0 = off); mainnet ships 16.

## v0.23.0 — reorg wired live: mainnet is now permissionless multi-validator

- **Gossip routed through the reorg engine** for postorage nodes: incoming blocks go through
  `AcceptBlock` (fork-choice + fast-forward/reorg) instead of the strict linear path. Verified live
  (3-node postorage: two validators rotate, V2 joined follower→validator, storage proofs pass).
- **Reorg survives restarts + side branches:** block bodies persisted by id (`store.PutBlock`), the
  replay base (genesis or sync checkpoint) persisted + reloaded; reorg replays from disk if needed.
  Fast-synced nodes get a **checkpoint** (their synced tip) as the reorg floor.
- **Fast-forward path:** linear head-extending blocks apply directly (no full replay) — only a
  divergent heavier branch triggers a rewind.
- **Mainnet flipped to `postorage`** (chainparams): the network is now **permissionless
  multi-validator** — any node with a bond + registration PoW joins the minting rotation, and the
  reorg engine keeps everyone converged. (Genesis/format unchanged → no reset; nodes auto-update.)

## v0.22.0 — reorg engine (state rewind + replay)

- **`chain.AcceptBlock`** — fork-choice-aware ingestion: accepts blocks on ANY known branch (not
  just head+1), adds them to the `forkchoice` tree, and when a competing branch becomes **heavier**
  performs a **REORG** — rewinds state to genesis and **replays the winning branch** onto a fresh
  copy, committing only if the whole branch applies cleanly (a bad branch can't corrupt live state).
  Consensus-safety capstone: nodes converge after a partition / equivocating producer.
- `state.Clone` (rewind snapshots); per-block tree + in-session body store; orphans (unknown parent)
  refused. Tested: two producers fork, the node sees branch A then heavier branch B, reorgs to B
  with B's exact state.
- Honest scope: v1 replays from the in-session genesis state, so live-gossip routing through
  `AcceptBlock` + persistent side-branch bodies + genesis-state for fast-synced nodes is the final
  integration step (mainnet stays on the linear authority path, untouched). The hard part — the
  fork-choice + rewind/replay algorithm — is done and tested.

## v0.21.0 — fork-choice rule (deterministic heaviest-chain selection)

- **`internal/forkchoice`** — a block tree + deterministic chain-selection rule: among competing
  branches pick the **heaviest cumulative weight**, ties broken by lowest block id, plus
  `CommonAncestor` (the rewind point). Every node computes the same canonical tip from the same tree
  → convergence without coordination, replacing "first-valid-block-per-height wins" (which could
  diverge under a partition). Per-block weight is pluggable — the intent is **storage/availability
  weight** so the heaviest chain is the most-available one (ShadowLedger's thesis as fork choice).
- This is the **selection rule** (tested). The chain-level **reorg** that acts on it — rewind state
  to the common ancestor and replay the heavier branch — is the next slice (needs per-block state
  snapshots / body replay). Security test (5 local nodes + remote): forged blocks rejected (409) and
  the attacker IP banned (403); no poison could be injected.

## v0.20.0 — registration Proof-of-Work (PoW × PoS Sybil gate)

- **`internal/regpow`** — to register as a validator you now solve a one-time PoW puzzle
  (`H(chainID‖address‖nonce)` with N leading zero bits) **on top of** the bond. This is the genuine
  PoW role (Zilliqa/QuarkChain style): **Sybil resistance** (mass fake validators cost real compute)
  + a **randomness seed** for shard assignment — NOT block mining (no wasted energy). Mainnet
  difficulty = 20 bits (~1M hashes, sub-second).
- `state` verifies the nonce on `KindRegister` (carried in tx `Data`); `slctl register` solves it
  automatically. Bound to chain id + address (can't be reused). `RegPoWBits` is a network param
  (0 = off, so tests/devnets can disable). Additive — no chain reset.
- Together: **PoW to enter** (anti-Sybil) + **bond/PoS for stake** + **storage to earn** (the ongoing
  competition is proven storage → leader weight, once on-chain proofs land).

## v0.19.0 — proof-of-availability (reconstruct + verify against commitment)

- **`chain.VerifyAvailable(height)` / `GET /verify/{height}` / `slctl verify`** — reconstructs a
  block's body from pooled shards (local + fetched) and checks it against the validator's signed
  `BodyHash` + `MerkleRoot`. A block that can't be reconstructed to its commitment is
  withheld/corrupt = NOT available. Building block for **availability-weighted fork choice**: the
  canonical chain is the one whose blocks are reconstructable from the fragment pool.
- docs/CONSENSUS.md documents the design (incl. the Reed-Solomon all-or-nothing fact: <K shards
  recover nothing — the pool must collectively hold K; there is no partial reconstruction).
- Additive — no chain reset. The full reorg engine is the remaining consensus work.

## v0.18.0 — VM byte layer (full addresses + strings → real ERC-721)

- **Byte-storage layer** in the VM: contracts now have `bkey → []byte` storage beside the uint64
  store, for **full addresses and strings**. New opcodes `BSTORE` (copy a calldata byte-range),
  `CALLERB`/`SELFB` (store a full address), `BEQ`/`BLEN`/`BHASH`, `RETURNB` (return bytes), `CDLEN`.
- **Full-address ERC-721** now possible: `ownerOf` returns the real owner address (not a uint64
  digest) and `transfer` compares the caller's full address. Token URIs/names are byte blobs.
  Tested end-to-end (`erc721_test.go`).
- `state.QueryContractRaw` + `POST /call` return `return_bytes` (hex). Account gains `BStorage`
  (covered by clone/rollback). Additive — **no chain reset** (tx/header format unchanged).
- Remaining: a standard ABI convention + `.shl` sugar for byte/string values (byte ops work from
  raw bytecode today).

## v0.17.0 — consensus liveness (leader-timeout / round fallback)

- **No more single-leader stalls.** The leader for a block is now elected per `(height, prevHash,
  round)`; `round` = time elapsed since the head / block-time. If the round-0 leader is offline for
  one block-time, round 1 elects a *different* validator who can produce — the chain keeps moving.
- Header gains a signed `Round` field; `ApplyExternalBlock` enforces **round-timing**
  (`ts >= prevTs + round*blockTime`, and not far in the future) so a validator can't jump to a high
  round to steal a slot early. Genesis now uses a fixed `GenesisTime` (chainparams) to anchor timing.
- Authority (single-validator mainnet) behavior unchanged. Honest limit: still *first-valid-block-
  per-height wins* (no reorg) — heaviest-chain fork choice is the next consensus item (docs/CONSENSUS.md).
- Header format changed → fresh chain.

## v0.16.0 — usable token/NFT/meme contracts + ChainID replay protection + security review

- **Usable example contracts in `.shl`**: [contracts/token.shl](contracts/token.shl) (ERC-20-style:
  init/transfer/balanceOf/totalSupply), [contracts/memecoin.shl](contracts/memecoin.shl) (fixed
  21M supply + burn), plus the existing NFT. Balances keyed by address digest. Tested end-to-end
  (deploy → init → transfer → overdraw no-op → burn → supply).
- **ChainID replay protection**: every tx now carries a `ChainID` bound into its signature; the
  state rejects txs from another network (`ErrBadChainID`). Mainnet ChainID = 1 (in chainparams);
  `slctl` stamps it automatically. (Tx format change → fresh chain.)
- **[docs/SECURITY.md](docs/SECURITY.md)**: honest security inventory, gaps by priority, threat
  notes, and a security-ordered roadmap. TL;DR: treat mainnet as a testnet until liveness/fork-choice
  and enforceable storage proofs land.

## v0.15.0 — SHL contract language + compiler + gas estimate

- **`.shl` high-level language** with a real lexer → parser → **AST** → codegen pipeline
  (`internal/shl`) targeting the VM. Has variables, arithmetic, comparisons, `&&`/`||`/`!`,
  **`if`/`else`, `while`**, `store[]`, builtins (`caller/value/balance/self/arg`), `emit`, `return`.
- **`slctl compile --in prog.shl [--out prog.hex]`** — source → bytecode (hex), ready for
  `slctl deploy --code`.
- **`slctl estimate --in prog.shl`** — approximate gas for one pass (mirrors the VM gas schedule;
  notes that loops are per-iteration and CALL callee gas is excluded).
- Example contracts [contracts/counter.shl](contracts/counter.shl) and
  [contracts/nft.shl](contracts/nft.shl); language reference [docs/SHL.md](docs/SHL.md).
- Tested: compile+run counter, if/else, while, range checks, gas estimate.

## v0.14.0 — hybrid log history (Ethereum root + ShadowLedger fragments)

- Logs are no longer stored whole on every node (that contradicted the no-node-stores-everything
  thesis). New hybrid model:
  - **Ethereum-style commitment**: a merkle **`LogsRoot`** over the block's logs is added to the
    header, signed and part of the block id. `ApplyExternalBlock` recomputes it from re-execution and
    **rejects forged event history** (`ErrBadLogsRoot`).
  - **ShadowLedger-style storage**: log data is erasure-coded + rendezvous-distributed like block
    bodies — no node holds all logs. `GET /logs/{height}` reconstructs from K-of-(K+M) log-shards and
    verifies against `LogsRoot` before returning.
- `Log` moved to `internal/types` with canonical encoding (`EncodeLogs`/`DecodeLogs`/`LogsRootOf`).
  Tested end-to-end (`TestLogsHybridRoundTrip`): emit → fragment → reconstruct → integrity-check.

## v0.13.0 — contract event logs (on-chain history)

- **`LOG` opcode** — contracts emit events (`pop n` + `n` topic words). Events are collected per
  block (including from nested cross-contract calls), discarded on revert, and persisted as block
  history. Deterministic and re-derivable by re-executing the block — real history, not a side DB.
- **`GET /logs/{height}` / `slctl logs --height N`** — query a block's events (the substrate
  indexers need for ERC-721 `Transfer`, etc.). Tested: the NFT now emits a Transfer event on mint.
- Gets ERC-721 most of the way: mint/transfer/ownerOf/**events** all work. Remaining for full
  ERC-721: full-address words + string metadata (next).

## v0.12.0 — read-only contract calls + deeper contract tests

- **`POST /call` / `slctl query`** — read-only contract execution (eth_call analog): runs a method
  against current state and returns its value, with no tx, no fee and no state mutation. Turns any
  contract into a read API (`ownerOf`, `balanceOf`, ...).
- **Deeper contract tests**: a working minimal **NFT** (mint / owner-gated transfer / ownerOf, with
  unauthorized transfer reverting) and explicit **gas-metering** tests (out-of-gas reverts; enough
  gas succeeds), built with a small label assembler.
- Docs: SMART-CONTRACTS.md now explains the gas model ("how do you buy gas" — you don't; gas is a
  compute budget, fee is flat), execution paths, and honest NFT/API capabilities + limits.

## v0.11.0 — versioning + voluntary updates

- **Build version** baked in (`internal/version`, injected from the git tag via release ldflags).
  `slnode --version`, `slctl version`, `GET /version`, and `/health` now report it.
- **Update awareness, never auto-apply:** a node checks GitHub for a newer release on startup and
  only **logs** "update available" — upgrades stay voluntary (no central kill switch), the Bitcoin
  model. See docs/CONSENSUS.md philosophy.
- **Opt-in self-updater for your own node** (`scripts/sl-update.sh` + systemd timer): pulls the
  latest release, **verifies SHA256 before swapping**, restarts the service. Removes manual redeploy
  for a node you operate; must not be run fleet-wide. See docs/DEPLOY.md.

## v0.10.0 — peer banlist + equivocation slashing

- **Local peer banlist** (`internal/p2p/banlist.go`): misbehaving peers accrue strikes (invalid
  gossip block → strike sender IP; holder serving a shard that fails its committed hash → strike that
  node id) and are temp-banned with exponential backoff; bans self-expire. Banned IPs are rejected at
  both HTTP channels; banned peers skipped in broadcast/discover/shard-fetch. `slctl bans` /
  `GET /bans`. Local DoS hygiene — not consensus.
- **On-chain equivocation slashing** (real economic security): new `KindSlash` tx carries two
  conflicting headers signed by the same validator at the same height (provable double-signing).
  Verified on-chain → the validator's **bond is burned** (10% bounty to the reporter), the validator
  is set inactive and **permanently barred** from re-registering. `slctl slash --evidence ev.json`.
- Honest scope: this slashes **equivocation** (verifiable with no extra infra). Slashing **failed
  storage proofs** still needs on-chain proof records (next). Fully tested.

## v0.9.0 — on-chain validator registry + bond (permissionless entry)

- **Validators are on-chain state** (`state.Validators`), not config. New tx kinds **register**
  (locks `Amount` as a bond ≥ `economy.MinBond` = 1,000 SHARD) and **unregister** (returns the
  bond). `slctl register --bond N` / `slctl unregister` / `slctl validators` / `GET /validators`.
- **PoStorage reads the live on-chain set** (`state.ActiveValidators`) — anyone who posts a bond
  joins the minting rotation with no config change; deterministic across nodes (no membership forks).
  Genesis seeds the registry with the founder. Verified live: a node registered via tx and appeared
  in the rotation.
- Registry changes are covered by the block's atomic rollback.
- Honest caveat (see docs/CONSENSUS.md): mainnet stays `authority` until a **leader-timeout/fork
  choice** exists (an offline elected validator currently stalls the chain) and bonds are **slashable**.

## v0.8.0 — PoStorage leader election (decentralized minting)

- **`PoStorage` consensus engine** (`internal/consensus`): one leader per height elected by HRW
  `SHA-256(prevHash ‖ addr ‖ height)` over the validator set. Minting **rotates** among validators
  instead of a single authority; deterministic from on-chain data so there's exactly one valid
  leader per height (no forks from disagreement). Genesis is exempt from election.
- Storage-eligibility gate (`pos.Scoreboard.Eligible`) wired as **advisory** — a node won't try to
  lead if it's failing storage proofs. Enforceable on-chain proofs + slashing are the documented
  next step.
- Engine interface generalized (`LeaderFor(height, prev)`, `CanProduce(next, prev)`, `IsValidator`);
  selectable via `consensus: authority|postorage` (default `authority`).
- New doc [docs/CONSENSUS.md](docs/CONSENSUS.md): who mints, why no PoW, and the honest gap to
  Bitcoin/Ethereum-grade security (permissionless entry, on-chain proofs, slashing, fork choice).

## v0.7.0 — zero-config mainnet

- **Embedded chain params** (`internal/chainparams`): genesis premine, validators and bootstrap
  seeds are baked into the binary (like Bitcoin/Ethereum). `slnode` with no config joins mainnet —
  derives the shared deterministic genesis, connects to seeds, fast-syncs. No `node.yaml` required.
- Config is now a partial override merged over the embedded params; data defaults to
  `~/.shadowledger`. `slctl` auto-targets a local node or falls back to the mainnet seed.
- **Bootstrap/validator node deployed** to a public VPS (`136.248.77.107`, the embedded seed).
- Docs: [docs/DEPLOY.md](docs/DEPLOY.md) (zero-config join + VPS hosting + cloud-firewall + SELinux).

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
