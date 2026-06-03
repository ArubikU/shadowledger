# Ficha de Investigación: ShadowLedger

## 1. Título del Proyecto
**ShadowLedger: una blockchain de historia fragmentada por codificación de borrado, donde el almacenamiento — no el cómputo — es el recurso escaso**

*(EN, para submission):* *ShadowLedger: A Storage-Scarce Blockchain with Erasure-Coded Fragmented History and Proof-of-Storage Consensus*

## 2. Investigador y Asesor
- **Estudiante:** [ANONIMIZAR — double-blind]
- **Asesor:** [ANONIMIZAR]
- **Institución:** Universidad de Lima (Facultad de Ingeniería y Arquitectura)
- **Target:** CIIS 2026 — ACM ICPS / Scopus — short paper (deadline 30 jun 2026)

## 3. Resumen del Problema
Las blockchains tradicionales obligan a cada nodo completo a almacenar **toda** la historia de la
cadena. Este costo de almacenamiento crece sin cota y de forma monótona, lo que centraliza la red
(solo operadores con mucho disco corren nodos completos) y desperdicia recursos por replicación
total. En paralelo, las cadenas tipo Bitcoin gastan energía masiva en Proof-of-Work (PoW) para
producir bloques. ShadowLedger ataca ambos: convierte el **almacenamiento** en el recurso escaso y
medible (en lugar del cómputo), y reparte la historia de modo que **ningún nodo guarda la cadena
completa**, manteniendo a la vez recuperabilidad, seguridad ante manipulación y entrada permissionless.

## 4. Objetivos del Estudio
- **General:** Diseñar, implementar y validar empíricamente una arquitectura de blockchain
  permissionless cuya historia esté fragmentada por codificación de borrado y cuyo consenso sustituya
  el PoW por prueba de almacenamiento (Proof-of-Storage).
- **Específicos:**
  1. Fragmentar cada bloque con Reed–Solomon (K+M) y distribuir los shards por *rendezvous hashing*,
     garantizando reconstrucción con cualquier K-de-(K+M) y nada con <K (todo-o-nada).
  2. Definir un consenso PoStorage con elección de líder verificable, registro de validadores
     on-chain con bond, gate anti-Sybil por PoW de registro y *slashing* por equivocación.
  3. Implementar un motor de reorganización (rewind + replay de la rama más pesada) y finality por
     profundidad que garantice irreversibilidad dura más allá de F bloques.
  4. Proveer una vía de incorporación de nuevos participantes sin capital previo (faucet por PoW) y
     una máquina virtual de contratos determinista con un lenguaje de alto nivel (SHL).
  5. Validar el sistema en un despliegue real (mainnet prototipo) y medir su comportamiento.

## 5. Temas Críticos a Tratar
- **Estado vs. Historia:** distinción central. El *estado* (saldos, nonces, storage de contratos) es
  acotado y derivable → se materializa local en cada nodo. La *historia* (cuerpos de bloque) es no
  acotada → es lo único que se fragmenta y dispersa. Esta separación es lo que hace viable "que nadie
  guarde todo" sin romper la validación.
- **Disponibilidad de datos (Data Availability):** la cadena canónica es la *reconstruible* desde los
  fragmentos agrupados; un bloque que no se reconstruye contra su compromiso firmado (BodyHash) se
  considera retenido/no disponible.
- **Seguridad ante envenenamiento:** todo campo del bloque va firmado y validado; cada shard va
  comprometido por hash. Un atacante externo no puede insertar un bloque; un validador no puede
  reescribir historia (cadena de prev-hash + firmas) ni acuñar fuera del cronograma.
- **PoW útil, no desperdiciado:** el único PoW es (a) gate de registro anti-Sybil y (b) faucet para
  followers, anclado al compromiso de un bloque reciente. La producción de bloques no quema energía.
- **Ética/sostenibilidad:** menor huella energética que PoW; descentralización por menor barrera de
  almacenamiento; mainnet tratada como testnet (sin valor real) hasta auditoría externa.

## 6. Metodología y Diseño de la Evaluación
Investigación de **diseño de sistemas** (Nivel 5): construcción de un artefacto + evaluación empírica
en despliegue. Implementación en Go; despliegue de un nodo bootstrap en VPS cloud + nodos locales.
Mediciones del prototipo:
- **Recuperabilidad:** reconstrucción de cuerpos desde K-de-(K+M) shards dispersos vs. compromiso firmado.
- **Resistencia a manipulación:** batería de 7 variantes de bloque forjado (firma inválida, validador
  no autorizado, merkle/body alterado, altura/prev incorrectos, timestamp futuro, logs root forjado).
- **Convergencia de consenso:** partición de 2 validadores → reorg (rewind+replay) → convergencia.
- **Finality:** verificar que ninguna reorg rebaja por debajo de head−F (F=16).
- **Onboarding sin capital:** wallet con saldo 0 obtiene $SHARD vía faucet PoW (costo en hashes medido).
- **Contratos:** desplegar y ejecutar un token (SHL) en mainnet (deploy → init → transfer → query).

## 7. Resultados (prototipo) y Esperanzas
> Mediciones de **un** despliegue prototipo, no generalización (calificar como tal).
1. **Funcionalidad end-to-end:** cadena viva (>8000 bloques, 5 s/bloque) en mainnet; suite de pruebas
   Go completa en verde (~16 paquetes).
2. **Seguridad:** 7/7 bloques forjados rechazados; convergencia de 2 validadores tras partición.
3. **Irreversibilidad:** reorg por debajo del piso de finality rechazada por construcción.
4. **Onboarding:** wallet vacío 0 → 2 $SHARD con ~2.6×10⁵ hashes (PoW faucet) en mainnet real.
5. **Contratos:** token SHL desplegado y operado en mainnet.

## 8. Brechas declaradas (honestidad académica)
- Fork-choice pesa hoy por **longitud** (no por storage probado); pesaje determinista requiere
  registros de prueba on-chain (trabajo futuro).
- **Sin auditoría externa**; sin benchmark multi-región ni a gran escala (N de nodos pequeño).
- Sin compromiso de *state root* en el header todavía (light-clients confían vía replay/snapshot).

## 9. Referencias Clave (34 VERIFICADAS — DOI/arXiv confirmados en Crossref/Semantic Scholar)
Bibliografía completa en `paper/references.bib` (paste-ready BibTeX, ACM-Reference-Format). Pilares:
- **Erasure / coded blockchains / DA:** Reed–Solomon (1960, DOI 10.1137/0108018); Perard et al. (2018, low-storage node); Kadhe et al. (SeF, arXiv:1906.12140); Yu et al. (Coded Merkle Tree, FC 2020); Al-Bassam et al. (Fraud & DA Proofs, FC 2021); Hall-Andersen et al. (Foundations of DAS, 2025); survey Yang et al. (ACM CSUR 2024, 10.1145/3637224).
- **Placement / sharding / fork-choice:** Thaler & Ravishankar (HRW, 1998, 10.1109/90.663936); Karger et al. (consistent hashing, STOC '97); Elastico / OmniLedger / RapidChain / Monoxide; GHOST (Sompolinsky & Zohar, FC 2015); Bitcoin Backbone (Garay et al., EUROCRYPT 2015); FlyClient (Bünz et al., IEEE S&P 2020).
- **Proof-of-Storage / Stake / slashing:** Proofs of Space (Dziembowski et al., CRYPTO 2015); SpaceMint (FC 2018); Permacoin (Miller et al., IEEE S&P 2014); PoRet (Juels & Kaliski, CCS '07); Filecoin PoRep; Ouroboros (CRYPTO 2017); Tendermint/Casper/Gasper.

> Gray literature (sin DOI, citar como @misc): Chia green paper, Filecoin scaling-PoRep, Storj WP.
> Pendiente camera-ready: confirmar nombres de pila + rangos de página contra PDFs del editor.
