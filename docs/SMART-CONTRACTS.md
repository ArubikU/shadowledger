# Smart contracts (v0.4)

ShadowLedger runs deterministic on-chain code via a minimal stack VM
(`internal/vm`). Determinism is total — no time, randomness, or floats — so every
node computes identical results, which consensus requires.

## Model

- A **contract account** is a normal account that additionally holds `Code` (bytecode) and
  `Storage` (uint64 → uint64 key/value).
- **Deploy** (`KindDeploy`): a tx whose `Data` is the bytecode. The contract address is
  `ContractAddress(deployer, nonce)` (deterministic, like Ethereum CREATE). v0.4 stores the code;
  no constructor is executed.
- **Call** (`KindCall`): a tx to a contract address; `Data` is the input (8-byte words), `Amount`
  is value sent, `Gas` bounds execution. Calling a non-contract is a plain transfer.
- **Gas & revert:** every opcode costs gas (SSTORE 100, SLOAD 20, PUSH 3, others 1). On any VM
  error (out-of-gas, div-by-zero, bad jump, stack under/overflow) the call **reverts**: storage is
  untouched and the sent value is refunded — but the **fee is still consumed** (anti-spam).
- State changes are atomic per block (full-account rollback snapshots).

## Instruction set

| Op | Hex | Effect |
|---|---|---|
| STOP | 0x00 | halt |
| PUSH | 0x01 | push next 8 bytes (big-endian) |
| POP | 0x02 | drop top |
| ADD SUB MUL DIV MOD | 0x10–0x14 | arithmetic (DIV/MOD by zero → revert) |
| EQ LT GT ISZERO AND OR NOT | 0x20–0x26 | comparison / bitwise |
| DUP SWAP | 0x30 0x31 | stack ops |
| JUMP JUMPI | 0x40 0x41 | unconditional / conditional jump |
| SLOAD SSTORE | 0x50 0x51 | storage read / write |
| CALLER VALUE BAL | 0x60–0x62 | caller-id, call value, contract balance |
| CDLOAD | 0x63 | push input word at popped index |
| SELF | 0x64 | push this contract's id |
| CALL | 0x71 | pop gasLimit, arg, target → call target contract; push its return (0 on fail) |
| LOG | 0x72 | pop n, pop n topic words → emit an event (recorded in block history) |
| RETURN | 0x70 | pop → return value, halt |

Stack is uint64 words (max 1024 deep); step limit 1,000,000.

## Example: a counter

`storage[0] = storage[0] + 1` each call:

```
PUSH 0      ; SSTORE key (sits under the value)
PUSH 0      ; SLOAD key
SLOAD       ; -> storage[0]
PUSH 1
ADD         ; -> storage[0]+1
SSTORE      ; storage[0] = storage[0]+1
STOP
```

Bytecode (hex):
```
010000000000000000010000000000000000500100000000000000011051 00
```
(spaces for readability; strip them.)

## Using it from the CLI

```
# write the hex above to counter.hex (no spaces), then:
slctl deploy --wallet w.tok --code counter.hex --gas 100000 --rpc http://localhost:4004
#   -> contract address: sl…

slctl call --wallet w.tok --to sl<contract> --gas 100000 --rpc http://localhost:4004
# inspect storage via the state snapshot:
curl -s http://localhost:4004/state/snapshot | jq '.accounts["sl<contract>"].storage'
# -> {"0": 1}
```

## Contract-to-contract calls (v0.5)

A contract can call another with the `CALL` opcode: push `target` (the callee's id — its
`AddrDigest`), one `arg` word, and a `gasLimit`, then `CALL`. The host resolves the id to a
contract, runs it with `arg` as a one-word input, and pushes the callee's `RETURN` value (or `0`
if the target is missing or reverted). Calls forward remaining gas, are depth-limited (8) to bound
recursion/reentrancy, and carry no value in v0.5. Nested storage changes commit only if the whole
transaction succeeds (block-level rollback covers them).

```
PUSH <callee-id>   ; target (AddrDigest of the callee)
PUSH <arg>         ; one input word
PUSH <gasLimit>    ; gas to forward
CALL               ; -> pushes callee return value (0 on failure)
```

## How gas works (and "how do you buy gas?")

ShadowLedger does **not** have a gas market (no `gasPrice`, no EIP-1559). Two separate things:

- **`Gas`** on a tx = an **execution budget** (a step/compute ceiling). Each opcode costs gas
  (`SSTORE` 100, `SLOAD` 20, `PUSH` 3, others 1). If a call exceeds its `Gas`, it **reverts**
  (out-of-gas) — this is the anti-infinite-loop guard, verified in tests. You don't *buy* gas; you
  set a high-enough limit (unused gas costs nothing).
- **`Fee`** = the flat amount paid to the block validator for including your tx. It is charged
  whether the call succeeds or reverts (anti-spam). The fee is **not** `gasUsed × price` today.

So: gas = "how much compute am I allowed," fee = "what I pay." A true `fee = gasUsed × gasPrice`
market is a deliberate future choice (see roadmap) — for now keep `Gas` generous and set a small
`Fee`.

## How a contract is executed

1. **Deploy** — `slctl deploy --code prog.hex` sends a `KindDeploy` tx (`Data` = bytecode). The
   contract address is `ContractAddress(deployer, nonce)`, deterministic.
2. **Call (writes)** — `slctl call --to <addr> --data <hex words> --gas N` sends a `KindCall` tx.
   Every node re-executes the bytecode against the contract's storage when applying the block, so
   the result is identical everywhere (consensus). Revert → storage untouched, value refunded, fee
   kept.
3. **Query (reads)** — `slctl query --to <addr> --data <hex>` hits `POST /call`: runs the contract
   **read-only** against current state and returns its `RETURN` value. No tx, no fee, no mutation —
   the analog of Ethereum's `eth_call`, for serving data.

Input is 8-byte big-endian **words**; `CDLOAD <i>` reads input word `i`, so a call can pass a
selector + arguments. `RETURN` yields one word.

## Can it do NFTs?

**A minimal NFT: yes** (tested — `TestNFTMintTransferOwnerOf`). A contract with selector dispatch:
`mint` sets `storage[tokenId] = CALLER`, `transfer` checks `storage[tokenId] == CALLER` then
reassigns (reverts on unauthorized), `ownerOf` returns the owner. Mint/transfer/access-control all
work, and `ownerOf` is queryable read-only.

**Events/history: done (v0.13).** The `LOG` opcode emits events recorded in block history; the
tested NFT emits a `Transfer`-style event on mint. Query them at `GET /logs/{height}` / `slctl logs
--height N`. Logs are deterministic and re-derivable by re-executing the block, so they're real
history, not a side database. (A `logsRoot` header commitment for light-client proofs is a later
add.)

**Still not full production ERC-721**, honestly:
- Owners are stored as a **uint64 digest** of the address, not the full address (VM words are
  uint64) — fine for ownership checks, but you can't recover the full owner address on-chain.
- **No string metadata** (token URIs) — storage is uint64→uint64.
- No standard ABI/interface.

Remaining for full ERC-721: address-width words (or byte arrays) + string storage. Events are now
in place.

## Can it do APIs?

Two senses:
- **Contracts as read APIs** — yes: `POST /call` / `slctl query` turns any contract method into a
  read endpoint (`ownerOf`, `balanceOf`, ...) with no tx. The node's HTTP RPC *is* the API surface.
- **Contracts calling external web APIs (oracles)** — **no, and never directly**: the VM is
  deterministic by design (no network, time, or randomness), because every node must compute the
  same result. External data must come through an **oracle pattern** (someone posts the data on-chain
  via a tx; the contract reads it). That's standard for all blockchains, not a ShadowLedger gap.

## Limits / roadmap

Still small: uint64-only words (so addresses are stored as digests), no events/logs, no string
storage, no value transfer in cross-contract `CALL`, no gas-price market, no constructor. Next:
address/byte-array values + an event/log opcode (unlocks real ERC-721/20), `fee = gasUsed × price`,
value-bearing calls, possibly a WASM backend (`wazero`). See [GAPS-AND-DESIGN.md](GAPS-AND-DESIGN.md).
