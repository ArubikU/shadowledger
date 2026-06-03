# MEMORY — ShadowLedger / CIIS 2026 paper

## 2026-06-03 — Ficha + short paper + research/benchmarks
### Decisiones
- Proyecto del paper = **ShadowLedger** (blockchain de historia fragmentada por erasure coding + Proof-of-Storage). PROJECT_CONTEXT.md viejo describía OTRO proyecto (SER/WhatsApp emoción) → reescrito.
- Target: **CIIS 2026**, ACM `acmart` sigconf, **inglés**, **double-blind** (`anonymous,review`), short paper 4–6 pp, deadline 30 jun 2026. Eje: seguridad/sistemas distribuidos + algoritmos.
- Paper = **systems paper** (prototipo corriendo en mainnet) → evaluación con mediciones reales, no propuesta.

### Hecho
- `PROJECT_CONTEXT.md` reescrito (ShadowLedger + requisitos CIIS).
- `ficha/ficha-shadowledger.md` — ficha de investigación (español).
- `paper/main.tex` — short paper ACM anonimizado; **compila a 4 págs** (toolchain pdflatex→bibtex→pdflatex×2). acmart v2.16 + ACM-Reference-Format presentes en MiKTeX.
- `paper/references.bib` — **34 referencias VERIFICADAS** (DOI/arXiv vía Crossref/Semantic Scholar), 0 citas indefinidas. Generadas por 3 sub-agentes de investigación en paralelo (erasure/DA, sharding/HRW/fork-choice, proof-of-storage/PoS).

### Mediciones reales (benchmark Go efímero `cmd/_bench`, ya borrado)
- **Storage/nodo vs replicación total** (bloques 1 MiB, rendezvous, 2000 bloques): n=32→7.1×, n=64→14×, n=128→**28×**, n=256→57× menos. Cae ~O(1/n); <n=4 la paridad domina (no gana). TABLA en el paper.
- **Erasure throughput** (K16M8): encode 0.3–1.4 GB/s, decode 0.17–0.47 GB/s; overhead fijo 1.5×.
- **Todo-o-nada:** K shards→recupera; K−1→falla ("not enough shards").
- **PoW faucet** (mediana 9 ids, ~2.5M h/s 1 core): bits18→2.7×10⁵ hashes/105 ms; +2 bits ≈ ×4. TABLA en el paper.

### Pendiente (camera-ready, no bloquea)
- [ ] Confirmar nombres de pila + páginas de refs contra PDFs del editor (algunos inferidos por Crossref).
- [ ] Códigos CCS reales (dl.acm.org/ccs), bloque de autores, ACM e-Rights/DOI al aceptar.
- [ ] Decidir: expandir a **full paper 10–14 pp** (lo que pide ICPS/Scopus) vs dejar short (póster/Interfases).
- [ ] Gray literature (Chia/Filecoin/Storj) → reviewers pueden objetar; sustitutos peer-reviewed ya citados (SpaceMint, Moran–Orlov).

### Aprendizajes
- Binarios Go son Windows-native: NO usar paths `/tmp/...` de bash con ellos (no resuelven). Paths relativos o Windows.
- Compilar ACM: copiar `references.bib` a `output/` antes de correr `bibtex main` ahí (output-directory).

## 2026-06-03 (cont.) — Full paper modular + expansión a 10pp
### Hecho
- Paper reescrito a FULL (era short 4pp): + System Overview/arch fig, State-vs-History, PoStorage consensus + 3 algoritmos (leader/finality/faucet), Storage Auditing, Threat Model + tabla + análisis disponibilidad, Implementation + tabla módulos, Evaluation (storage/throughput/PoW/reconstrucción/comparison), Discussion, Design Rationale, Conclusion.
- **Modularizado**: `paper/secciones/01..15 + 06b` con `\input`; `main.tex` solo preamble+abstract+CCS+input list. `compile-all.ps1` reescrito (era de AVE) → pdflatex→bibtex→pdflatex×2, copia .bib, reporta pages+undefined.
- **34 refs verificadas** (3 sub-agentes research en paralelo, DOI/arXiv Crossref/S2) en `references.bib`; las 34 citadas (Storj era la única faltante, agregada en §02).
- **Em-dashes eliminados** (user: "avoid the usage of this") — `---`→`, ` global, 0 en fuente. Celdas tabla `---` → `n/a`.
- Benchmarks reales (cmd/_bench efímero, borrado): storage/nodo n=128→28×, erasure 0.3-1.4 GB/s overhead 1.5×, todo-o-nada K/K-1, PoW faucet bits18=2.7e5 hashes/105ms. 2 tablas + curva pgfplots en el paper.
- Baseline comparison añadido (full-node vs 3×-repl vs ShadowLedger): ataca el punto débil "sin baseline" del peer-review.
- **10 páginas**, 0 undefined, compila. Objetivo CIIS 10-14 → en rango (mínimo).
### Viabilidad (peer-review propio): ~73-76/100 Borderline/Weak-Accept para Scopus/LNCS (CIIS ICPS). Débil: calidad experimental (1 despliegue, sin multi-región).
## 2026-06-03 (cont.) — Review externo (76/100 Weak-Accept) + fixes + 12pp
Review crítico del usuario (PC-level). Críticas P1 y fixes aplicados:
- **Overclaim "Proof-of-Storage Consensus"** (prototipo = HRW uniforme + scoreboard advisory, sin weighting/slashing) → FIX: cláusula "implementation status" en abstract + contribución 2 con caveat + párrafo "what is built vs specified" en §6 (built: registry/bond/regpow/uniform-HRW/audit/slashing-equiv/reorg/finality; specified: storage-weighted election + missed-proof slashing, requieren on-chain storage-proof records).
- **Colusión rompe independencia de f** → FIX: párrafo "Correlated and colluding holders" en §10 (HRW reparte por identidad no por operador; regpow sube pero no elimina; declarado como limitación real).
- **VM/SHL = inflación** → FIX: §9 reframe como "supporting capability, not a scientific contribution... reader may skip".
- Calidad experimental 6/10 (un despliegue, sin testbed común, sintético) → ya declarado honestamente; queda como future work (benchmark multi-sistema/geo + churn).
Paper a 12pp (HRW formal eq, opcode table, reorg fig+ejemplo, param-sensitivity, reproducibility, future-work, org roadmap). 34/34 refs, 0 em-dash, 0 undefined.

## 2026-06-03 (cont.) — Fixes (a)+(b) del review: retítulo + storage-weighted consensus REAL
- (a) **Retítulo**: "...and Proof-of-Storage Consensus" → "**...and Storage-Weighted Consensus**" (matiza overclaim).
- (b) **CÓDIGO v0.27.0**: storage-weighted consensus construido + testeado + pusheado a GitHub.
  - `KindStorageProof` tx (height‖shardBytes); índice DERIVADO = H(addr‖blockID)%T (no cherry-pick).
  - Verificación on-chain vs ShardHash comprometido (store.GetHeader) — sin body, sin header change, SIN RESET.
  - `state.StorageWeights` (1+min(score,cap)); `PoStorage.LeaderFor` con copias virtuales (entero determinista) → elección ponderada por storage probado; decae a base si deja de probar.
  - Loop auto-prueba en node (validador postorage). chainparams StorageWindow=256/MinDepth=4/Cap=64.
  - Tests: state/storageproof_test (credita/bad/stale/decay) + consensus (65:1→>90%). 17 paquetes verdes.
- **Paper releído + flipeado** "specified"→"built" en TODOS los puntos (abstract, §1 contrib, §6 election+status, §6b audit, §7 fork, §10 grinding/colusión, §12 result, §13, §15). DISTINCIÓN clave mantenida honesta: elección ponderada=CONSTRUIDA; fork-choice-weight (peso de RAMA) + slashing-de-BOND = aún futuro. 12pp, 0 undefined, 0 em-dash.
- **VPS deploy v0.27.0 PENDIENTE**: VPS inalcanzable (HTTP+SSH timeout, blip transitorio recurrente). §12 redactado para NO afirmar observación en vivo (solo integración + "gated on rollout"). Reintentar deploy cuando VPS vuelva → entonces observar storage_score>0 en /validators.

### Pendiente
- [ ] **Desplegar v0.27.0 a VPS** cuando vuelva (SSH) + verificar storage proof aceptado on-chain (storage_score sube en /validators) → cierra el end-to-end live.
- [ ] (opcional) Retitular si se quiere matizar más el overclaim (hoy resuelto vía honestidad en cuerpo, sin retítulo).
- [ ] Subir a Accept: implementar storage-proof records on-chain (→ weighting + slashing reales) + testbed distribuido (50-100 nodos geo, churn). Eso cierra los 2 gaps que bajan de Q1.
- [ ] Crecer a 11-13pp cómodo (no solo 10) si se quiere margen.
- [ ] Camera-ready: autores reales, CCS reales (dl.acm.org/ccs), e-Rights/DOI al aceptar, quitar `anonymous,review`.
- [ ] Turnitin SOLO institucional (iThenticate ULima) — NO sitios "free" (confidencialidad + retención = self-plagio).
- [ ] Verificar nombres de pila/páginas de refs contra PDFs editor.
