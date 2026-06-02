# ShadowLedger — design answers & gap analysis (v0.2)

Direct answers to the open questions, plus what's done vs. still missing.

## 1. Proof-of-Work? — No. Here's why.

ShadowLedger's whole pitch is the opposite of Bitcoin's energy model. PoW exists to
manufacture artificial scarcity of *block-production rights* by burning electricity. We don't
need that:

- Block-production rights come from **identity + role** (v0 authority validator) and will grow
  into **Proof-of-Storage** — you earn the right to produce by *usefully storing shards*, which
  is work the network needs anyway.
- Adding PoW would re-introduce the exact waste (mining farms, ASICs, megawatts) the project
  set out to eliminate, and would contradict "run a node on a phone."

**What people actually want from "the Bitcoin pattern" is the *monetary policy*, not PoW.**
That part — fixed supply + halving — is fully decoupled from how blocks are produced, and it's
now implemented (see §2).

> If a future version wants permissionless leader election without authority, the right tool is
> Proof-of-Storage or Proof-of-Stake, not PoW.

## 2. $SHARD minting / recompensa — Bitcoin-style, implemented

`internal/economy`:

| Param | Value | Bitcoin analog |
|---|---|---|
| base unit | 1 SHARD = 100,000,000 units | sats |
| `MaxSupply` | 21,000,000 SHARD | 21M BTC |
| `InitialReward` | 50 SHARD/block | 50 BTC |
| `HalvingInterval` | 210,000 blocks | 210,000 |

- Every block carries a **coinbase tx (index 0)** = `block subsidy + fees`, paid to the block's
  **validator** (no miner). Subsidy halves every 210k blocks; clamps so emission never exceeds
  21M. **Fees are recycled, not minted** (don't inflate supply).
- Genesis premine counts toward the cap. `state.Minted` tracks total emission deterministically;
  every node recomputes the same expected reward and **rejects** any block whose coinbase amount
  violates the schedule (`ErrBadCoinbase`).
- Inspect live: `GET /supply` → `{minted, next_reward}`; `slctl supply`.

## 3. Peer discovery without a central server

There is **no central server**. A node finds the network three ways, all decentralized:

1. **Seeds (bootstrap entry points).** `seeds:` in config is a list of *control URLs* of any
   already-running nodes. These are NOT a central authority — they're just "someone you can ask
   first," exactly like Bitcoin's DNS seeds / hardcoded seed nodes. Anyone can be a seed; you can
   point at a friend's node, a community node, your own second machine, whatever.
2. **Peer exchange (PEX / gossip).** On `POST /hello`, a node sends its own descriptor and gets
   back *everyone the peer knows*. So contacting one seed transitively reveals the whole reachable
   graph. The `Discover()` loop re-runs every 20s, so the peer set converges and self-heals.
3. **LAN multicast (optional "scan").** With `lan_discovery: true`, nodes beacon their descriptor
   on UDP multicast `239.255.42.99:48999` and handshake with anyone they hear — zero-config
   discovery for machines on the same network, no seeds needed. (Internet peers still need a seed
   as the first hop; multicast doesn't cross routers.)

So: **it gossips, and optionally scans the LAN — it does not poll a central registry.**

## 4. How does someone turn their computer into a node?

```
# 1. install / build the binaries
go build -o slnode ./cmd/slnode
go build -o slctl ./cmd/slctl

# 2. make a minimal config pointing at ANY existing node as a seed
cat > node.yaml <<EOF
data_dir: ./sl-data
control_addr: ":4004"
shard_addr:   ":4005"
advertise: "your-host-or-ip"          # how peers reach you
validators:                            # who you accept blocks from (the network's validators)
  - sl<validator-address>
seeds:
  - http://some-existing-node:4004     # entry point; not central
lan_discovery: true                    # also auto-find peers on your LAN
EOF

# 3. run it — first launch auto-creates your node identity keypair
./slnode --config node.yaml
```

On startup the node: creates its identity key if absent → discovers peers via seeds/LAN →
**fast-syncs** the verified header chain + a state snapshot from the best peer → then participates
(stores its rendezvous-assigned shards from new blocks, serves shards, relays gossip). No genesis
config needed for a joiner — it gets the chain from the network (§5).

## 5. Late-joiner sync (new in v0.2)

A fresh node with no local history:
- `GET /headers?from=0&to=N` → full header chain; the joiner **verifies every header's validator
  signature and the `PrevHash` links**, so it cannot be fed a forged history.
- `GET /state/snapshot` → account balances + nonces + minted at the tip.
- `chain.SyncInstall` stores the verified headers and installs the snapshot.

**Trust note (honest):** the header chain is fully verified; the *state snapshot* is currently
trusted from the serving peer (assumeutxo-style). A fully trustless joiner would instead
reconstruct every block body from shards and replay. A future header `StateRoot` commitment would
make the snapshot directly verifiable — tracked below.

## 6. The network decides scale, not the operator (new in v0.2)

Per your requirement: **K/M shards and replication are not in any config.** `internal/netparams`
derives them from the live node count:

- `Spec(n)` → total shards scale with `n` (clamped 3..24), ~1/3 parity (tolerate ~⅓ holder loss).
  The producer stamps the chosen `(K,M)` into each block header; everyone else just reads it.
- `Replication(n)` → 1 (n<3), 2 (n<8), 3 (n≥8). Computed locally from each node's membership view.
- Genesis uses a **fixed** shape (`GenesisSpec`) so block 0 is identical everywhere.

As nodes join/leave, future blocks automatically re-shape; old blocks keep the shape recorded in
their header. The network self-tunes.

## 7. Gap analysis — done vs. missing

**Done (v0 + v0.2):** Ed25519 identity, tx/block/merkle, Reed-Solomon erasure + shard-hash
integrity, rendezvous placement, bloom index, account state, mempool, deterministic genesis,
authority consensus, HTTP control+shard channels, on-demand reconstruction, **fixed-cap + halving
emission**, **adaptive K/M/replication**, **seed+PEX+LAN discovery**, **late-joiner fast sync**,
shard serving across nodes.

**Still missing (prioritized):**
1. **Smart contracts** — not started. Recommendation: defer to v0.4; design a minimal deterministic
   VM (account + simple opcode interpreter, or WASM via `wazero`) *after* consensus hardens.
   Bolting on a VM now would destabilize the parts that just started working.
2. **Proof-of-Storage challenges** — the rendezvous assignment already binds nodes to shards;
   missing is the challenge/response (`prove you hold (block,index)` via random byte-range + shard
   Merkle path) and reward/slashing weighting.
3. **Multi-validator / fork choice** — v0 assumes a single producer; no BFT, no fork resolution.
4. **Trustless state sync** — add a `StateRoot` (Merkle/Verkle of accounts) to the header so the
   sync snapshot is verifiable instead of trusted.
5. **Active shard repair / re-replication** — when holders leave, no background job re-pushes shards
   to new top-R holders; reconstruction still works until too many leave, but durability decays.
6. **Wire format** — HTTP/JSON now; protobuf/gRPC + libp2p later for efficiency and NAT traversal.
7. **Encrypted shards** — v0 shards are plaintext erasure fragments (privacy is future).
8. **Mempool/DoS hardening, fee market, peer scoring/bans.**

## 8. Suggested next milestone (v0.3)

Pick the load-bearing gaps, in order: (a) `StateRoot` in header → trustless sync; (b) active shard
re-replication on churn → real durability; (c) Proof-of-Storage challenge → replace authority.
Smart contracts (v0.4) come after the ledger is durable and trust-minimized.
