// Package node wires storage, state, consensus, chain, mempool and networking
// into a runnable ShadowLedger daemon.
package node

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ArubikU/shadowledger/internal/bloom"
	"github.com/ArubikU/shadowledger/internal/chain"
	"github.com/ArubikU/shadowledger/internal/chainparams"
	"github.com/ArubikU/shadowledger/internal/consensus"
	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/mempool"
	"github.com/ArubikU/shadowledger/internal/p2p"
	"github.com/ArubikU/shadowledger/internal/state"
	"github.com/ArubikU/shadowledger/internal/store"
	"github.com/ArubikU/shadowledger/internal/types"
	"github.com/ArubikU/shadowledger/internal/version"
)

// Node is a running daemon.
type Node struct {
	cfg       *Config
	id        *crypto.KeyPair
	store     *store.Store
	state     *state.State
	chain     *chain.Chain
	pool      *mempool.Pool
	srv       *p2p.Server
	peers     *p2p.Peerstore
	eng       consensus.Engine
	bloom     *bloom.Filter
	statePath string

	ctrlHTTP  *http.Server
	shardHTTP *http.Server
}

// New constructs a node from config.
func New(cfg *Config) (*Node, error) {
	st, err := store.Open(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	id, err := crypto.LoadOrCreateWalletAuto(cfg.NodeKey, os.Getenv("SL_WALLET_PASS"))
	if err != nil {
		return nil, err
	}
	statePath := filepath.Join(cfg.DataDir, "state.json")
	ledger := state.New()
	if loaded, err := state.Load(statePath); err == nil {
		ledger = loaded
	}
	ledger.SetChainID(chainparams.Mainnet().ChainID) // bind txs to this network
	bf, err := bloom.Load(filepath.Join(cfg.DataDir, "bloom.dat"))
	if err != nil {
		bf = bloom.New(100000, 0.01)
	}

	var eng consensus.Engine
	switch cfg.Consensus {
	case "postorage":
		// Validator set is read LIVE from the on-chain registry, so registrations
		// change the minting rotation with no config edits. Storage gate is
		// advisory for now (on-chain proofs make it enforceable). docs/CONSENSUS.md.
		eng = consensus.NewPoStorage(ledger.ActiveValidators, id.Address(), nil, 0.8)
	default:
		eng = consensus.NewAuthority(cfg.Validators, id.Address())
	}
	pool := mempool.New(ledger, 50000)

	self := p2p.Peer{
		ID:      string(id.Address()),
		Control: "http://" + cfg.Advertise + cfg.ControlAddr,
		Shard:   "http://" + cfg.Advertise + cfg.ShardAddr,
	}
	peers := p2p.NewPeerstore(self)

	n := &Node{
		cfg: cfg, id: id, store: st, state: ledger, pool: pool,
		peers: peers, eng: eng, bloom: bf, statePath: statePath,
	}

	ch := chain.New(st, ledger, eng, bf, chain.Config{
		SelfID:  string(id.Address()),
		Members: peers.MemberIDs, // live, growing membership for rendezvous
	})
	n.chain = ch

	n.srv = p2p.NewServer(peers, cfg.Seeds, cfg.DNSSeeds, cfg.ControlAddr, ch, pool)
	ch.SetSource(n.srv)
	return n, nil
}

// Address returns the node identity address.
func (n *Node) Address() crypto.Address { return n.id.Address() }

// bootstrapGenesis deterministically derives block 0 from the genesis funding
// config. Because Genesis is deterministic, every node that shares the same
// genesis + validators config computes an identical genesis locally — no gossip
// of block 0 needed. A node WITHOUT genesis config instead syncs it from peers.
func (n *Node) bootstrapGenesis() error {
	if n.store.HasHeaders() || len(n.cfg.Genesis) == 0 {
		return nil
	}
	if len(n.cfg.Validators) == 0 {
		return errNoValidators
	}
	genesisVal := n.cfg.Validators[0]
	funding := map[crypto.Address]uint64{}
	for a, amt := range n.cfg.Genesis {
		funding[crypto.Address(a)] = amt
	}
	blk, err := n.chain.Genesis(funding, genesisVal, n.id)
	if err != nil {
		return err
	}
	_ = n.state.Save(n.statePath)
	log.Printf("genesis derived: height=0 id=%x txs=%d", idShort(blk.Header.ID()), len(blk.Txs))
	return nil
}

var errNoValidators = errString("config: at least one validator address required")

type errString string

func (e errString) Error() string { return string(e) }

// Run starts servers, discovery, sync and the production loop until ctx is done.
func (n *Node) Run(ctx context.Context) error {
	if err := n.bootstrapGenesis(); err != nil {
		return err
	}

	// Bind on all interfaces (":port") so the node is reachable from the public
	// internet when the operator forwards/maps the port. Timeouts guard against
	// slow-loris and hung peers on an open network.
	n.ctrlHTTP = &http.Server{
		Addr: n.cfg.ControlAddr, Handler: n.srv.ControlHandler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second,
	}
	n.shardHTTP = &http.Server{
		Addr: n.cfg.ShardAddr, Handler: n.srv.ShardHandler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 60 * time.Second, IdleTimeout: 90 * time.Second,
	}
	go serve("control/RPC", n.cfg.ControlAddr, n.ctrlHTTP)
	go serve("shard transfer", n.cfg.ShardAddr, n.shardHTTP)

	// Informational update check only — ShadowLedger NEVER auto-applies updates
	// (auto-update would be a central kill switch; upgrades are voluntary).
	go n.checkUpdate()

	// Decentralized discovery: resolve DNS seeds + contact seeds, exchange peers.
	if len(n.cfg.Seeds) > 0 || len(n.cfg.DNSSeeds) > 0 {
		if learned := n.srv.Discover(); learned > 0 {
			log.Printf("discovery: learned %d peer(s) from seeds", learned)
		}
	}

	// Late-joiner fast sync from the best peer (if any and we are behind).
	n.trySync()

	n.loops(ctx)
	return n.shutdown()
}

func serve(name, addr string, srv *http.Server) {
	log.Printf("%s listening on %s", name, addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("%s server: %v", name, err)
	}
}

func (n *Node) trySync() {
	_, hasGen := n.chain.Head()
	to, err := n.srv.SyncFrom(n.chain.Height(), hasGen)
	if err != nil {
		log.Printf("sync: %v", err)
		return
	}
	if to > 0 {
		_ = n.state.Save(n.statePath)
		log.Printf("synced to height=%d from peer (supply=%d)", to, n.state.Supply())
	}
}

func (n *Node) loops(ctx context.Context) {
	block := time.NewTicker(time.Duration(n.cfg.BlockTimeMS) * time.Millisecond)
	defer block.Stop()
	discover := time.NewTicker(20 * time.Second)
	defer discover.Stop()
	save := time.NewTicker(10 * time.Second)
	defer save.Stop()
	audit := time.NewTicker(15 * time.Second)
	defer audit.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-discover.C:
			n.srv.Discover()
			if !n.eng.IsValidator() {
				n.trySync() // followers keep catching up
			}
		case <-save.C:
			n.persist()
		case <-audit.C:
			if p, m := n.srv.AuditRound(); p+m > 0 {
				log.Printf("proof-of-storage: %d passed, %d missed", p, m)
			}
		case <-block.C:
			n.maybeProduce()
		}
	}
}

func (n *Node) maybeProduce() {
	if !n.eng.CanProduce(n.chain.Height()+1, n.chain.HeadID()) {
		return
	}
	txs := n.pool.Reap(1000)
	reward := n.state.NextReward(n.chain.Height() + 1)
	if len(txs) == 0 && reward == 0 {
		return // nothing to do and no subsidy to mint
	}
	blk, set, err := n.chain.ProduceBlock(txs, n.id)
	if err != nil {
		log.Printf("produce block: %v", err)
		return
	}
	n.pool.Remove(txs)
	_ = n.state.Save(n.statePath)
	n.srv.BroadcastBlock(blk)
	log.Printf("produced block height=%d txs=%d spec=K%dM%d nodes=%d id=%x",
		blk.Header.Height, len(blk.Txs), set.Spec.K, set.Spec.M, n.peers.Count(), idShort(blk.Header.ID()))
}

// checkUpdate logs (only) if a newer release exists. No auto-apply.
func (n *Node) checkUpdate() {
	latest, err := version.LatestRelease(8 * time.Second)
	if err != nil {
		return
	}
	if version.IsNewer(latest, version.Version) {
		log.Printf("update available: %s (running %s) — upgrade is voluntary; see %s/releases",
			latest, version.Version, "https://github.com/"+version.Repo)
	}
}

func (n *Node) persist() {
	_ = n.state.Save(n.statePath)
	_ = n.bloom.Save(filepath.Join(n.cfg.DataDir, "bloom.dat"))
}

func (n *Node) shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	n.persist()
	if n.ctrlHTTP != nil {
		_ = n.ctrlHTTP.Shutdown(ctx)
	}
	if n.shardHTTP != nil {
		_ = n.shardHTTP.Shutdown(ctx)
	}
	return nil
}

func idShort(h types.Hash) []byte { return h[:6] }
