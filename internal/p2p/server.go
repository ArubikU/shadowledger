package p2p

import (
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strconv"

	"github.com/ArubikU/shadowledger/internal/chain"
	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/pos"
	"github.com/ArubikU/shadowledger/internal/types"
	"github.com/ArubikU/shadowledger/internal/version"
)

// ControlHandler returns the HTTP mux for the control/RPC channel (:4004).
func (s *Server) ControlHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "id": s.store.Self().ID, "peers": s.store.Count() - 1, "version": version.Version})
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"version": version.Version, "repo": version.Repo})
	})
	mux.HandleFunc("GET /head", func(w http.ResponseWriter, r *http.Request) {
		hdr, ok := s.chain.Head()
		if !ok {
			http.Error(w, "no genesis", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, hdr)
	})
	mux.HandleFunc("GET /account/{addr}", func(w http.ResponseWriter, r *http.Request) {
		addr := crypto.Address(r.PathValue("addr"))
		ac := s.chain.State().Get(addr)
		writeJSON(w, map[string]any{"address": addr, "balance": ac.Balance, "nonce": ac.Nonce})
	})
	mux.HandleFunc("GET /supply", func(w http.ResponseWriter, r *http.Request) {
		st := s.chain.State()
		writeJSON(w, map[string]any{"minted": st.Supply(), "next_reward": st.NextReward(s.chain.Height() + 1)})
	})
	// Proof-of-Storage scoreboard: per-node challenge pass/miss history.
	mux.HandleFunc("GET /storage/scores", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.scores.Snapshot())
	})
	// On-chain validator registry (who can mint, with their bonds).
	mux.HandleFunc("GET /validators", func(w http.ResponseWriter, r *http.Request) {
		st := s.chain.State()
		out := map[string]any{}
		for _, a := range st.ActiveValidators() {
			vi, _ := st.ValidatorInfo(a)
			out[string(a)] = vi
		}
		writeJSON(w, out)
	})
	mux.HandleFunc("GET /peers", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"self": s.store.Self(), "peers": s.store.Peers(), "count": s.store.Count()})
	})
	// Decentralized discovery: handshake + peer exchange. The caller posts its
	// own descriptor; we record it and return everyone we know.
	mux.HandleFunc("POST /hello", func(w http.ResponseWriter, r *http.Request) {
		var p Peer
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.store.Add(p)
		writeJSON(w, helloReply{Self: s.store.Self(), Peers: s.store.Peers()})
	})
	mux.HandleFunc("GET /block/{height}", func(w http.ResponseWriter, r *http.Request) {
		h, err := strconv.ParseUint(r.PathValue("height"), 10, 64)
		if err != nil {
			http.Error(w, "bad height", http.StatusBadRequest)
			return
		}
		hdr, set, err := s.chain.HeaderAt(h)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{"header": hdr, "shard_set": set})
	})
	// Header chain for late-joiner sync: GET /headers?from=0&to=N
	mux.HandleFunc("GET /headers", func(w http.ResponseWriter, r *http.Request) {
		from, _ := strconv.ParseUint(r.URL.Query().Get("from"), 10, 64)
		to, err := strconv.ParseUint(r.URL.Query().Get("to"), 10, 64)
		if err != nil {
			to = s.chain.Height()
		}
		var recs []chain.HeaderRecord
		for h := from; h <= to; h++ {
			hdr, set, err := s.chain.HeaderAt(h)
			if err != nil {
				break
			}
			recs = append(recs, chain.HeaderRecord{Header: hdr, ShardSet: set})
		}
		writeJSON(w, recs)
	})
	// Contract event logs at a height (history for indexers / ERC-721 Transfer).
	mux.HandleFunc("GET /logs/{height}", func(w http.ResponseWriter, r *http.Request) {
		h, err := strconv.ParseUint(r.PathValue("height"), 10, 64)
		if err != nil {
			http.Error(w, "bad height", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(s.chain.Logs(h))
	})
	// Account-state snapshot at the chain tip (assumeutxo-style fast sync).
	mux.HandleFunc("GET /state/snapshot", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.chain.State())
	})
	// Proof-of-Availability: reconstruct the block from pooled shards and check
	// it against the validator's signed commitment.
	mux.HandleFunc("GET /verify/{height}", func(w http.ResponseWriter, r *http.Request) {
		h, err := strconv.ParseUint(r.PathValue("height"), 10, 64)
		if err != nil {
			http.Error(w, "bad height", http.StatusBadRequest)
			return
		}
		avail, err := s.chain.VerifyAvailable(h)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{"height": h, "available": avail})
	})
	mux.HandleFunc("GET /reconstruct/{height}", func(w http.ResponseWriter, r *http.Request) {
		h, err := strconv.ParseUint(r.PathValue("height"), 10, 64)
		if err != nil {
			http.Error(w, "bad height", http.StatusBadRequest)
			return
		}
		txs, err := s.chain.ReconstructBody(h)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"height": h, "tx_count": len(txs), "txs": txs})
	})
	// Read-only contract query (eth_call analog): execute a call against current
	// state, return its value, mutate nothing. Body: {"to","data"(hex),"caller","gas"}.
	mux.HandleFunc("POST /call", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			To     string `json:"to"`
			Data   string `json:"data"`
			Caller string `json:"caller"`
			Gas    uint64 `json:"gas"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data, _ := hex.DecodeString(req.Data)
		var caller uint64
		if req.Caller != "" {
			caller = types.AddrDigest(crypto.Address(req.Caller))
		}
		ret, retb, ok := s.chain.State().QueryContractRaw(crypto.Address(req.To), caller, data, req.Gas)
		writeJSON(w, map[string]any{"ok": ok, "return": ret, "return_bytes": hex.EncodeToString(retb)})
	})
	// Direct user submission: pool + gossip.
	mux.HandleFunc("POST /tx", func(w http.ResponseWriter, r *http.Request) {
		var tx types.Transaction
		if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.pool.Submit(tx); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		go s.BroadcastTx(tx)
		writeJSON(w, map[string]any{"accepted": true, "txid": hex.EncodeToString(hashOf(tx))})
	})
	// Gossip endpoints: accept without re-broadcast to avoid loops.
	mux.HandleFunc("POST /gossip/tx", func(w http.ResponseWriter, r *http.Request) {
		var tx types.Transaction
		if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = s.pool.Submit(tx) // best-effort
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("POST /gossip/block", func(w http.ResponseWriter, r *http.Request) {
		var blk types.Block
		if err := json.NewDecoder(r.Body).Decode(&blk); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.chain.ApplyExternalBlock(&blk); err != nil {
			// Only strike the sender for a genuine FORGERY — not for benign gossip
			// races (out-of-order height, prev mismatch) which honest peers hit.
			if chain.IsForgery(err) {
				if banned := s.bans.Strike(clientIP(r)); banned {
					http.Error(w, "banned", http.StatusForbidden)
					return
				}
			}
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		s.pool.Remove(blk.Txs)
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("GET /bans", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.bans.Snapshot())
	})
	return limitBody(s.banGate(mux), 64<<20) // 64 MiB cap + banlist gate
}

// banGate rejects requests from currently-banned IPs.
func (s *Server) banGate(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.bans.Banned(clientIP(r)) {
			http.Error(w, "banned", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// limitBody caps request body size to guard an internet-facing node.
func limitBody(h http.Handler, max int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, max)
		h.ServeHTTP(w, r)
	})
}

// ShardHandler returns the HTTP mux for the shard-transfer channel (:4005).
func (s *Server) ShardHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /shard/{block}/{index}", func(w http.ResponseWriter, r *http.Request) {
		raw, err := hex.DecodeString(r.PathValue("block"))
		if err != nil || len(raw) != 32 {
			http.Error(w, "bad block id", http.StatusBadRequest)
			return
		}
		idx, err := strconv.Atoi(r.PathValue("index"))
		if err != nil {
			http.Error(w, "bad index", http.StatusBadRequest)
			return
		}
		var blockID types.Hash
		copy(blockID[:], raw)
		data, err := s.chain.Store().GetShard(blockID, idx)
		if err != nil {
			http.Error(w, "not held", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(data)
	})
	// Proof-of-Storage: H(nonce || shardBytes) if this node holds the shard.
	mux.HandleFunc("GET /prove/{block}/{index}/{nonce}", func(w http.ResponseWriter, r *http.Request) {
		raw, err := hex.DecodeString(r.PathValue("block"))
		if err != nil || len(raw) != 32 {
			http.Error(w, "bad block id", http.StatusBadRequest)
			return
		}
		idx, err := strconv.Atoi(r.PathValue("index"))
		if err != nil {
			http.Error(w, "bad index", http.StatusBadRequest)
			return
		}
		nraw, err := hex.DecodeString(r.PathValue("nonce"))
		if err != nil || len(nraw) != 32 {
			http.Error(w, "bad nonce", http.StatusBadRequest)
			return
		}
		var blockID, nonce types.Hash
		copy(blockID[:], raw)
		copy(nonce[:], nraw)
		data, err := s.chain.Store().GetShard(blockID, idx)
		if err != nil {
			http.Error(w, "not held", http.StatusNotFound)
			return
		}
		proof := pos.Challenge(nonce, data)
		w.Write([]byte(hex.EncodeToString(proof[:])))
	})
	mux.HandleFunc("GET /have/{block}", func(w http.ResponseWriter, r *http.Request) {
		raw, err := hex.DecodeString(r.PathValue("block"))
		if err != nil || len(raw) != 32 {
			http.Error(w, "bad block id", http.StatusBadRequest)
			return
		}
		var blockID types.Hash
		copy(blockID[:], raw)
		writeJSON(w, map[string]any{"have": s.chain.Store().HeldShards(blockID)})
	})
	return s.banGate(mux)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func hashOf(tx types.Transaction) []byte {
	h := tx.Hash()
	return h[:]
}
