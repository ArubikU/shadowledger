package node

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/ArubikU/shadowledger/internal/chainparams"
	"github.com/ArubikU/shadowledger/internal/crypto"
)

// Config is the node daemon configuration (YAML).
//
// NOTE: erasure parameters (K/M) and shard replication are NOT configured here.
// The network decides them adaptively from the live node count (package
// netparams). Operators only choose identity, endpoints, who they trust as
// validators, and how to find the network.
type Config struct {
	DataDir     string            `yaml:"data_dir"`
	ControlAddr string            `yaml:"control_addr"` // e.g. ":4004"
	ShardAddr   string            `yaml:"shard_addr"`   // e.g. ":4005"
	Advertise   string            `yaml:"advertise"`    // host others reach us at (default localhost)
	NodeKey     string            `yaml:"node_key"`     // path to identity wallet json
	Validators  []crypto.Address  `yaml:"validators"`   // authorized producer addresses
	Seeds       []string          `yaml:"seeds"`        // bootstrap control URLs (entry points)
	DNSSeeds    []string          `yaml:"dns_seeds"`    // DNS seed hostnames (A/AAAA list live node IPs)
	Consensus   string            `yaml:"consensus"`    // "authority" (default) | "postorage"
	BlockTimeMS int               `yaml:"block_time_ms"`
	Genesis     map[string]uint64 `yaml:"genesis"` // address -> funding (producer bootstrap)
}

// LoadConfigOrMainnet loads the config at path, or — if it does not exist —
// returns the embedded mainnet config so `slnode` runs with ZERO setup (like
// `bitcoind` connecting to Bitcoin mainnet out of the box). A partial config is
// merged over the embedded network params, so an operator only overrides what
// they care about (e.g. node_key + advertise for the validator node).
func LoadConfigOrMainnet(path string) (*Config, error) {
	if _, err := os.Stat(path); err != nil {
		return MainnetConfig(), nil
	}
	c, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	c.mergeMainnet()
	return c, nil
}

// MainnetConfig is the fully-defaulted embedded mainnet configuration.
func MainnetConfig() *Config {
	c := &Config{}
	c.mergeMainnet()
	c.applyDefaults()
	return c
}

// mergeMainnet fills empty network-identity fields from the baked-in params.
func (c *Config) mergeMainnet() {
	p := chainparams.Mainnet()
	if len(c.Validators) == 0 {
		c.Validators = p.Validators
	}
	if len(c.Seeds) == 0 {
		c.Seeds = p.Seeds
	}
	if len(c.DNSSeeds) == 0 {
		c.DNSSeeds = p.DNSSeeds
	}
	if len(c.Genesis) == 0 {
		c.Genesis = map[string]uint64{}
		for a, amt := range p.Genesis {
			c.Genesis[string(a)] = amt
		}
	}
	if c.ControlAddr == "" {
		c.ControlAddr = p.ControlAddr
	}
	if c.ShardAddr == "" {
		c.ShardAddr = p.ShardAddr
	}
	if c.BlockTimeMS == 0 {
		c.BlockTimeMS = p.BlockTimeMS
	}
	c.applyDefaults()
}

// LoadConfig reads and defaults a config file.
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	c.applyDefaults()
	return &c, nil
}

func defaultDataDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".shadowledger")
	}
	return "./sl-data"
}

func (c *Config) applyDefaults() {
	if c.DataDir == "" {
		c.DataDir = defaultDataDir()
	}
	if c.ControlAddr == "" {
		c.ControlAddr = ":4004"
	}
	if c.ShardAddr == "" {
		c.ShardAddr = ":4005"
	}
	if c.NodeKey == "" {
		c.NodeKey = c.DataDir + "/keys/node.json"
	}
	if c.Advertise == "" {
		c.Advertise = "localhost"
	}
	if c.Consensus == "" {
		c.Consensus = "authority"
	}
	if c.BlockTimeMS == 0 {
		c.BlockTimeMS = 5000
	}
}
