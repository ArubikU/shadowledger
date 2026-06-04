package p2p

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/ArubikU/shadowledger/internal/chain"
	"github.com/ArubikU/shadowledger/internal/erasure"
	"github.com/ArubikU/shadowledger/internal/netparams"
	"github.com/ArubikU/shadowledger/internal/pos"
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
		if s.bans.Banned(h) {
			continue
		}
		url := s.store.shardURLFor(h)
		if url == "" {
			continue
		}
		data, err := s.get(fmt.Sprintf("%s/shard/%s/%d", url, idHex, index))
		if err != nil {
			lastErr = err
			continue
		}
		// Verify against the committed shard hash; strike holders serving garbage.
		if !erasure.VerifyShard(set, index, data) {
			s.bans.Strike(h)
			lastErr = fmt.Errorf("p2p: holder %s served invalid shard %d", short(h), index)
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

// BroadcastBlock gossips a full block to all known (non-banned) peers.
func (s *Server) BroadcastBlock(blk *types.Block) {
	for _, p := range s.store.Peers() {
		if s.bans.Banned(p.ID) {
			continue
		}
		_ = s.postJSON(p.Control+"/gossip/block", blk, nil)
	}
}

// BroadcastTx gossips a tx to all known (non-banned) peers.
func (s *Server) BroadcastTx(tx types.Transaction) {
	for _, p := range s.store.Peers() {
		if s.bans.Banned(p.ID) {
			continue
		}
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

// resolveDNSSeeds turns DNS seed hostnames into control URLs. Each seed's
// A/AAAA records list live node IPs (Bitcoin's DNS-seed model); we assume the
// conventional control port for them.
func (s *Server) resolveDNSSeeds() []string {
	var urls []string
	for _, host := range s.dnsSeeds {
		ips, err := net.LookupHost(host)
		if err != nil {
			continue
		}
		for _, ip := range ips {
			if h, _, e := net.SplitHostPort(ip); e == nil {
				ip = h // LookupHost shouldn't include a port, but be safe
			}
			if isIPv6(ip) {
				urls = append(urls, fmt.Sprintf("http://[%s]%s", ip, s.dnsPort))
			} else {
				urls = append(urls, fmt.Sprintf("http://%s%s", ip, s.dnsPort))
			}
		}
	}
	return urls
}

func isIPv6(ip string) bool {
	p := net.ParseIP(ip)
	return p != nil && p.To4() == nil
}

// Discover runs one discovery round: resolve DNS seeds, then handshake with all
// seeds and known peers, absorbing any new peers they report (peer exchange).
// No central server is involved — seeds are just entry points.
func (s *Server) Discover() int {
	targets := map[string]bool{}
	for _, seed := range s.seeds {
		targets[seed] = true
	}
	for _, url := range s.resolveDNSSeeds() {
		targets[url] = true
	}
	for _, p := range s.store.Peers() {
		if s.bans.Banned(p.ID) {
			continue
		}
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

// BestPeerHeight returns the greatest head height among known peers (0 if none
// reachable). Used to suppress block production while behind, so a validator that
// fell behind re-syncs instead of extending a stale side fork.
func (s *Server) BestPeerHeight() uint64 {
	var best uint64
	for _, p := range s.store.Peers() {
		if h, err := s.remoteHead(p.Control); err == nil && h.Height > best {
			best = h.Height
		}
	}
	return best
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

// AuditRound runs one Proof-of-Storage challenge round. For each shard this
// node holds at the current head, it challenges the OTHER assigned holders
// (co-holders) with a fresh random nonce and verifies their proof against the
// bytes it holds. Honest holders score a pass; missing/wrong/timeout score a
// miss. Returns (passes, misses) issued this round.
func (s *Server) AuditRound() (int, int) {
	head, ok := s.chain.Head()
	if !ok {
		return 0, 0
	}
	blockID := head.ID()
	members := s.MemberIDs()
	repl := netparams.Replication(len(members))
	self := s.store.Self().ID
	idHex := hex.EncodeToString(blockID[:])
	passes, misses := 0, 0

	for _, idx := range s.chain.Store().HeldShards(blockID) {
		data, err := s.chain.Store().GetShard(blockID, idx)
		if err != nil {
			continue
		}
		for _, holder := range rendezvous.Holders(members, blockID, idx, repl) {
			if holder == self {
				continue
			}
			url := s.store.shardURLFor(holder)
			if url == "" {
				continue
			}
			var nonce types.Hash
			rand.Read(nonce[:])
			want := pos.Challenge(nonce, data)
			got, perr := s.proveShard(url, idHex, idx, nonce)
			if perr == nil && got == want {
				s.scores.Pass(holder)
				passes++
			} else {
				s.scores.Miss(holder)
				misses++
			}
		}
	}
	return passes, misses
}

// proveShard asks a holder for H(nonce || shardBytes) over the shard channel.
func (s *Server) proveShard(shardURL, blockHex string, index int, nonce types.Hash) (types.Hash, error) {
	var out types.Hash
	b, err := s.get(fmt.Sprintf("%s/prove/%s/%d/%s", shardURL, blockHex, index, hex.EncodeToString(nonce[:])))
	if err != nil {
		return out, err
	}
	raw, err := hex.DecodeString(string(bytes.TrimSpace(b)))
	if err != nil || len(raw) != 32 {
		return out, fmt.Errorf("p2p: bad proof response")
	}
	copy(out[:], raw)
	return out, nil
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
