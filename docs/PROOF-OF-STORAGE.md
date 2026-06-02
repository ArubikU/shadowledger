# Proof-of-Storage (v0.6)

ShadowLedger replaces Proof-of-Work's burned electricity with **proof that you store useful data**:
the erasure shards rendezvous hashing assigned to you. Nodes that can't prove possession lose
standing (and, in the multi-validator model, block-production rights and recycled-fee share).

## The challenge

```
proof = SHA-256( nonce || shardBytes )
```

A challenger picks a **fresh random 32-byte nonce** and asks a holder for the proof of a specific
shard. Because the nonce is unpredictable, the holder cannot precompute or cache an answer — it must
have the actual bytes at challenge time. The challenger, which holds the same shard (it only audits
shards it itself stores → co-holders), recomputes `H(nonce ‖ ourBytes)` and compares.

- Match → `Pass` for that holder.
- Wrong / timeout / 404 → `Miss`.

Endpoint (shard channel): `GET /prove/{blockID}/{shardIndex}/{nonceHex}` → returns the proof hex if
the node holds that shard, else `404`.

## Scoreboard

`internal/pos.Scoreboard` keeps per-node `{pass, miss, last_seen}`. `Ratio()` = pass / (pass+miss).
`Eligible(id, minRatio)` reports whether a node clears a threshold — never-challenged nodes get the
benefit of the doubt. Query it:

```
slctl storage --rpc http://localhost:4004
# {"sl<id>":{"pass":12,"miss":0,"last_seen":1780403458}, ...}
```

The auditor runs every ~15s: for each shard this node holds at the current head, it challenges the
other assigned holders. With the network-decided replication factor ≥ 2 (≥3 nodes) there are
co-holders to audit; below that there is nothing to challenge.

## What's wired vs. next

- **Wired:** the challenge protocol, random-nonce verification, the scoreboard, the `/prove`
  endpoint, the periodic auditor, and `Eligible()`.
- **Next (multi-validator):** actually *gate* validator selection and fee distribution on the
  scoreboard, and slash persistent missers. Under the v0 single-authority producer there is one
  validator, so eligibility is informational today — but the scoreboard is the substrate the
  multi-validator upgrade builds on. See [GAPS-AND-DESIGN.md](GAPS-AND-DESIGN.md).
