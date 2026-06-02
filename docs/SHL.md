# SHL — the ShadowLedger contract language

`.shl` is a tiny high-level language that **compiles to ShadowLedger VM bytecode** via a real
lexer → parser → AST → codegen pipeline (`internal/shl`). It exists so you write `if`/`while`/
expressions instead of hand-assembling opcodes.

```
slctl compile  --in prog.shl --out prog.hex   # .shl -> bytecode (hex)
slctl estimate --in prog.shl                  # approx gas for one pass
slctl deploy   --wallet w.tok --code prog.hex --rpc <node>
```

## Language

A program is a list of statements (the contract body, run on each call). Values are `uint64`.

**Statements**
```
let x = <expr>;            // declare/assign a variable (storage-backed, see note)
x = <expr>;                // reassign
store[<expr>] = <expr>;    // write contract storage (SSTORE)
if (<expr>) { ... } else { ... }
while (<expr>) { ... }
emit(<expr>, <expr>, ...); // event log; args become topics (LOG)
return <expr>;             // halt, return a value (queryable read-only)
stop;                      // halt
```

**Expressions**
```
numbers           42
variables         x
storage read      store[k]
builtins          caller()  value()  balance()  self()  arg(i)
arithmetic        + - * / %
comparison        == != < > <= >=
logic             && || !
grouping          ( ... )
```

`arg(i)` reads the i-th 8-byte word of call input — use `arg(0)` as a method selector.

## Example: counter

```
store[0] = store[0] + 1;
return store[0];
```

## Example: NFT (selector dispatch)

```
if (arg(0) == 0) {                       // mint
    store[arg(1)] = caller();
    emit(0, caller(), arg(1));
} else {
    if (arg(0) == 1) {                   // transfer (owner-gated)
        if (store[arg(1)] == caller()) {
            store[arg(1)] = arg(2);
            emit(1, caller(), arg(2), arg(1));
        } else { return 0; }
    } else {                             // ownerOf
        return store[arg(1)];
    }
}
```

See [contracts/counter.shl](../contracts/counter.shl) and [contracts/nft.shl](../contracts/nft.shl).

## Gas estimate

`slctl estimate` sums each opcode's cost over one straight-line pass (mirrors the VM schedule:
SSTORE 100, LOG 50+8/topic, SLOAD 20, PUSH 3, others 1). It's an approximate **guide**, not exact:
`while` loops cost per-iteration (counted once), and cross-contract `CALL` does not include the
callee's gas. Set the tx `--gas` comfortably above the estimate.

## Notes / limits (v1 compiler)

- **Variables are storage-backed** (high reserved slots `0xFFFFFFFF00000000+`), so they persist
  across calls — initialize before use. (A future version may use a non-persistent scratch region.)
- Values are `uint64`; addresses appear as their digest (same limit as the VM — see
  [SMART-CONTRACTS.md](SMART-CONTRACTS.md)).
- No functions yet (single program body), no strings. Control flow, storage, events, builtins and
  arithmetic are supported.
