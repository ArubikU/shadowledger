package p2p

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ArubikU/shadowledger/internal/chain"
	"github.com/ArubikU/shadowledger/internal/netparams"
	"github.com/ArubikU/shadowledger/internal/rendezvous"
	"github.com/ArubikU/shadowledger/internal/state"
	"github.com/ArubikU/shadowledger/internal/types"
)

// FetchShard implements chain.ShardSource: it pulls shard `index` of a block
// from the rendezvous-ranked holders. The replication factor (how many holders
// to consider) is the network-decided value for the current node count. The
// chain verifies the bytes against the committed shard hash, so a lying peer is
// simply skipped here.
func (s *Server) FetchShard(blockID types.Hash, index int, set types.ShardSet) ([]byte, error) {
	members := s.MemberIDs()
	repl := netparams.Replication(len(members))
	holders := rendezvous.Holders(members, blockID, index, repl)
	idHex := hex.EncodeToString(blockID[:])
	var lastErr error
	for _, h := range holders {
		url := s.store.shardURLFor(h)
		if url == "" {
			continue
		}
		data, err := s.get(fmt.Sprintf("%s/shard/%s/%d", url, idHex, index))
		if err != nil {
			lastErr = err
			continue
		}
		return data, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("p2p: no holder served shard %d", index)
	}
	return nil, lastErr
}

func (s *Server) get(url string) ([]byte, error) {
	resp, err := s.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("p2p: GET %s -> %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (s *Server) postJSON(url string, in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	resp, err := s.client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("p2p: POST %s -> %d", url, resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

// BroadcastBlock gossips a full block to all known peers' control channels.
func (s *Server) BroadcastBlock(blk *types.Block) {
	for _, p := range s.store.Peers() {
		_ = s.postJSON(p.Control+"/gossip/block", blk, nil)
	}
}

// BroadcastTx gossips a tx to all known peers' control channels.
func (s *Server) BroadcastTx(tx types.Transaction) {
	for _, p := range s.store.Peers() {
		_ = s.postJSON(p.Control+"/gossip/tx", tx, nil)
	}
}

// helloReply is what /hello returns: the responder's identity + its known peers.
type helloReply struct {
	Self  Peer   `json:"self"`
	Peers []Peer `json:"peers"`
}

// sayHello performs a handshake + peer-exchange against a control URL: it tells
// the peer who we are and learns the peer plus everyone the peer knows. This is
// the decentralized discovery primitive — no central directory involved.
func (s *Server) sayHello(controlURL string) (int, error) {
	var reply helloReply
	if err := s.postJSON(controlURL+"/hello", s.store.Self(), &reply); err != nil {
		return 0, err
	}
	learned := 0
	if s.store.Add(reply.Self) {
		learned++
	}
	learned += s.store.Merge(reply.Peers)
	return learned, nil
}

// Discover runs one discovery round: handshake with all seeds and known peers,
// absorbing any new peers they report (transitive peer exchange).
func (s *Server) Discover() int {
	targets := map[string]bool{}
	for _, seed := range s.seeds {
		targets[seed] = true
	}
	for _, p := range s.store.Peers() {
		targets[p.Control] = true
	}
	total := 0
	for url := range targets {
		if n, err := s.sayHello(url); err == nil {
			total += n
		}
	}
	return total
}

// remoteHead fetches a peer's current head header.
func (s *Server) remoteHead(controlURL string) (types.Header, error) {
	b, err := s.get(controlURL + "/head")
	if err != nil {
		return types.Header{}, err
	}
	var h types.Header
	return h, json.Unmarshal(b, &h)
}

// fetchHeaders downloads the header chain [0..to] from a peer.
func (s *Server) fetchHeaders(controlURL string, to uint64) ([]chain.HeaderRecord, error) {
	b, err := s.get(fmt.Sprintf("%s/headers?from=0&to=%d", controlURL, to))
	if err != nil {
		return nil, err
	}
	var recs []chain.HeaderRecord
	return recs, json.Unmarshal(b, &recs)
}

// fetchSnapshot downloads a peer's account-state snapshot.
func (s *Server) fetchSnapshot(controlURL string) (*state.State, error) {
	b, err := s.get(controlURL + "/state/snapshot")
	if err != nil {
		return nil, err
	}
	snap := state.New()
	if err := json.Unmarshal(b, snap); err != nil {
		return nil, err
	}
	return snap, nil
}

// SyncFrom fast-syncs the local chain from the best-known peer if it is ahead.
// Returns the height synced to (0 and nil if already current / nothing to do).
func (s *Server) SyncFrom(localHeight uint64, hasGenesis bool) (uint64, error) {
	best := ""
	var bestHeight uint64
	for _, p := range s.store.Peers() {
		h, err := s.remoteHead(p.Control)
		if err != nil {
			continue
		}
		if h.Height >= bestHeight {
			bestHeight, best = h.Height, p.Control
		}
	}
	if best == "" {
		return 0, nil // no reachable peers
	}
	if hasGenesis && bestHeight <= localHeight {
		return 0, nil // already current
	}
	recs, err := s.fetchHeaders(best, bestHeight)
	if err != nil {
		return 0, err
	}
	snap, err := s.fetchSnapshot(best)
	if err != nil {
		return 0, err
	}
	if err := s.chain.SyncInstall(recs, snap); err != nil {
		return 0, err
	}
	return bestHeight, nil
}
