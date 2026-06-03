# SHL — the ShadowLedger contract language

`.shl` is a high-level language that **compiles to ShadowLedger VM bytecode** via a real
lexer → parser → AST → codegen pipeline (`internal/shl`). It comes in two shapes from one grammar:

- **Solidity-like** (recommended): `state` variables + mappings, `fn` functions with automatic
  **selector dispatch**, `require`/`revert`, `msg.sender`, and overflow-checked arithmetic.
- **Flat script** (original): a bare list of statements run top-to-bottom, dispatching by hand on
  `arg(0)`. Still fully supported — good for one-liners.

```
slctl compile  --in prog.shl --out prog.hex   # .shl -> bytecode (hex)
slctl estimate --in prog.shl                  # approx gas for one pass
slctl deploy   --wallet w.tok --code prog.hex --rpc <node>
slctl call     --wallet w.tok --to <c> --fn transfer --args <to>,<amount> --rpc <node>
slctl query    --to <c> --fn balanceOf --args <who> --rpc <node>     # read-only
```

## Solidity-like dialect

```
state balances;            // a named storage variable / mapping (compiler-assigned slot)
state totalSupply;

fn transfer(to, amount) {  // a function; calldata word 0 selects it, words 1.. are params
    require(balances[msg.sender] >= amount);
    balances[msg.sender] = balances[msg.sender] - amount;
    balances[to] = balances[to] + amount;
    emit(1, msg.sender, to, amount);
}

fn balanceOf(who) { return balances[who]; }
```

**`state` declarations** replace magic numbers. Each name gets a fixed storage slot. Use it as a
scalar (`totalSupply = 1000;`, `return totalSupply;`) or as a **mapping** (`balances[k]`), where the
storage key is `MIX(slot, k)` — the VM's keccak-equivalent — so two mappings never collide.

**`fn` functions** are dispatched by selector. The selector of a function is `be8(sha256(name))`
(the analogue of Solidity's 4-byte keccak selector). A caller puts the selector in calldata word 0;
the compiler emits a dispatcher that matches it, binds the parameters from words 1, 2, … , and
**reverts on no match**. `slctl call --fn NAME --args a,b` computes the selector for you.

**`require(cond);`** reverts the whole call (discarding all storage writes) if `cond` is zero —
unlike the old `return 0;` no-op. **`revert;`** aborts unconditionally.

**`msg.sender` / `msg.value`** are aliases for `caller()` / `value()`.

**Arithmetic is overflow-checked by default** (Solidity 0.8 semantics): `+ - *` revert on
overflow/underflow instead of wrapping; `/ %` revert on a zero divisor.

## Statements

```
state name;                // (top level) named storage var / mapping
fn name(p1, p2) { ... }    // (top level) function with selector dispatch
let x = <expr>;            // declare/assign a local (storage-backed, see notes)
x = <expr>;                // assign a local OR a state scalar
name[<expr>] = <expr>;     // mapping write
store[<expr>] = <expr>;    // raw storage write (SSTORE) — flat style
require(<expr>);           // revert if zero
revert;                    // unconditional revert
if (<expr>) { ... } else { ... }
while (<expr>) { ... }
emit(<expr>, ...);         // event log; args become topics (LOG)
return <expr>;             // halt, return a value (queryable read-only)
stop;                      // halt
```

## Expressions

```
numbers           42
variables         x                 (param / local / state scalar)
mapping read      balances[k]
raw storage read  store[k]
builtins          caller() value() balance() self() arg(i)
msg fields        msg.sender  msg.value
arithmetic        + - * / %          (overflow-checked)
comparison        == != < > <= >=
logic             && || !
grouping          ( ... )
```

`arg(i)` reads the i-th 8-byte word of call input directly (flat style; in `fn` form the params do
this for you).

## Example: ERC-20 token

```
state balances;
state totalSupply;
state initialized;

fn init() {
    require(initialized == 0);
    initialized = 1;
    balances[msg.sender] = 1000000;
    totalSupply = 1000000;
    emit(0, msg.sender, 1000000);
}
fn transfer(to, amount) {
    require(balances[msg.sender] >= amount);
    balances[msg.sender] = balances[msg.sender] - amount;
    balances[to] = balances[to] + amount;
    emit(1, msg.sender, to, amount);
}
fn balanceOf(who) { return balances[who]; }
fn supply() { return totalSupply; }
```

See [token.shl](../contracts/token.shl), [nft.shl](../contracts/nft.shl),
[memecoin.shl](../contracts/memecoin.shl). The original flat style still works —
[counter.shl](../contracts/counter.shl):

```
store[0] = store[0] + 1;
return store[0];
```

## Gas estimate

`slctl estimate` sums each opcode's cost over one straight-line pass (mirrors the VM schedule:
SSTORE/BSTORE 100, LOG 50+8/topic, SLOAD/MIX 20, PUSH 3, others 1). It's an approximate **guide**,
not exact: `while` loops cost per-iteration (counted once), and cross-contract `CALL` does not
include the callee's gas. Set the tx `--gas` comfortably above the estimate.

## How close to Solidity is it? (honest)

Close in **shape**, not in **scale**. You get: named state + mappings, functions with selector
dispatch + params, `require`/`revert`, checked math, `msg.sender`, events. You do **not** get
Solidity's 256-bit integers (SHL is `uint64`), dynamic arrays/structs/strings as first-class types
(addresses/strings live in the VM byte layer — see [SMART-CONTRACTS.md](SMART-CONTRACTS.md)),
inheritance, modifiers, or the ABI. It's a faithful subset for tokens, NFTs and access-controlled
logic — not a drop-in Solidity replacement.

## Notes / limits (compiler)

- **Locals are storage-backed** (reserved high slots `0xFFFFFFFF00000000+`); `state` vars sit at
  `0xFFFFFFF000000000+`. Both persist across calls — initialize before use.
- Values are `uint64`. Full addresses/strings use the VM byte layer (CALLERB/BSTORE/RETURNB), not yet
  surfaced as SHL syntax — drop to hand-assembled bytecode for those (see the ERC-721 example test).
