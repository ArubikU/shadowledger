# Contexto del Proyecto: ShadowLedger

> Cargar este archivo al inicio de sesión junto con `.claude/CLAUDE.md`.

## Identidad
- **Proyecto:** ShadowLedger — blockchain con historia fragmentada por codificación de borrado (erasure-coded fragmented history), sin Proof-of-Work para producir bloques.
- **Institución:** Universidad de Lima, Facultad de Ingeniería y Arquitectura
- **Autores:** [ANONIMIZAR en submission — double-blind]
- **Target de publicación:** CIIS 2026 (IX Congreso Internacional de Ingeniería de Sistemas)

## Target CIIS 2026 (Call for Papers)
| Requisito | Valor |
|---|---|
| Idioma | **Inglés** únicamente |
| Plantilla | ACM (TAPS) — `acmart`, `\documentclass[sigconf,anonymous,review]{acmart}` |
| Extensión paper completo | 10–14 págs estilo ACM (incl. referencias) |
| Short paper (este draft) | 4–6 págs de contenido + referencias |
| Revisión | **Doble ciego** → eliminar nombres, afiliación, IP de VPS, repo/handle de GitHub, metadatos |
| Originalidad | Inédito, no en revisión simultánea |
| Indexación | ACM ICPS → Scopus (acceso abierto, APC cubierto) |
| Deadline envío | **30 junio 2026** |
| Eje temático | Ciencias de la computación (estructuras de datos y algoritmos) + Seguridad, privacidad y ética en sistemas |

## Tesis central (la contribución)
El almacenamiento — no el cómputo — es el recurso escaso. Ningún nodo guarda la cadena completa:
cada bloque se fragmenta con Reed–Solomon en K+M shards distribuidos por **rendezvous hashing (HRW)**;
cualquier K-de-(K+M) reconstruye el cuerpo (todo-o-nada: <K shards no recuperan nada). La producción
de bloques **no usa PoW** (sería desperdicio energético); el derecho a minar viene de **probar que
almacenas** los fragmentos asignados (Proof-of-Storage) sobre un registro de validadores con bond.

## Componentes técnicos (implementado en Go, corriendo en mainnet)
- **Erasure layer:** Reed–Solomon (`klauspost/reedsolomon`); K/M adaptativo según nº de nodos (`netparams`); placement por HRW.
- **Consenso:** PoStorage — elección de líder por `HRW(prevHash, height, round)` sobre el registro on-chain de validadores. Fallback de liveness por rondas (líder offline → siguiente ronda).
- **Sybil/seguridad de entrada:** registro = bond (PoS) + PoW de registro de un solo uso (`regpow`, gate anti-Sybil). Equivocation slashing (doble-firma quema el bond).
- **Fork-choice + reorg:** árbol de bloques, cadena más pesada gana; rewind de estado + replay de la rama ganadora. **Finality** por profundidad (F=16): el replay-base avanza a head−F y nunca retrocede ⇒ irreversibilidad dura.
- **$SHARD:** cap 21M, halving cada 210k bloques, coinbase al validador. **Faucet PoW** para followers: minar `H(chainID‖addr‖anchorBodyHash‖nonce)` con N ceros líderes; pago desde tesoro (no nuevo minteo).
- **Contratos / VM:** lenguaje SHL (subset estilo Solidity: state vars + mappings, funciones con dispatch por selector, require/revert, aritmética con chequeo de overflow) → bytecode de máquina de pila uint64 determinista.
- **Estado vs historia:** el estado (saldos/nonces/storage) se materializa local (recurso acotado, derivable); la **historia** (cuerpos) es lo que se fragmenta (recurso no acotado).
- **Red:** descubrimiento por DNS-seed + peer-exchange (sin servidor central). Despliegue real: nodo bootstrap en VPS cloud; cadena viva > 8000 bloques, 5 s/bloque.

## Evidencia de prototipo (resultados medibles, para la sección de evaluación)
> Calificar SIEMPRE como mediciones de **un despliegue prototipo**, no generalización estadística (Nivel 5: diseño/propuesta de sistema).
- Faucet PoW: ~2.6×10^5 hashes (bits=18) por claim; wallet vacío 0 → 2 $SHARD probado en mainnet real.
- Reorg: convergencia de 2 validadores tras partición (rewind+replay) verificada.
- Anti-envenenamiento: 7 variantes de bloque forjado rechazadas (firma, validador, merkle, altura, timestamp, logs root).
- Suite Go completa en verde (~16 paquetes con tests).
- Contrato token SHL desplegado y ejecutado en mainnet (deploy + init + transfer + query).

## Honestidad académica (gaps a declarar en el paper, no ocultar)
- Fork-choice hoy pesa por **longitud de cadena** (weight=1); el pesaje por storage probado requiere registros de prueba on-chain (trabajo futuro).
- **Sin auditoría externa de seguridad**; mainnet tratada como testnet (sin valor real).
- Evaluación = un despliegue, no benchmark multi-región ni N grande.

## Compilador LaTeX
```powershell
$exe = "$env:USERPROFILE\scoop\apps\latex\current\texmfs\install\miktex\bin\x64\pdflatex.exe"
# Paper ACM: cd paper && & $exe -interaction=nonstopmode -output-directory=..\output main.tex (3 pasadas + bibtex)
# Output: D:\Github\ShadowLedger\Tesis\output\
```
