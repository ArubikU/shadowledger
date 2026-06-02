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

## Limits / roadmap

Still small: single uint64 arg/return across calls (no byte-array ABI), no value transfer in CALL,
no dynamic memory, no events/logs, uint64-only words, no constructor. Next: value-bearing calls,
multi-word calldata/return, event logs, richer types, possibly a WASM backend (`wazero`). See
[GAPS-AND-DESIGN.md](GAPS-AND-DESIGN.md).
