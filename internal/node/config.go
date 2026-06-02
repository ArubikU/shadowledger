package node

import (
	"os"

	"gopkg.in/yaml.v3"

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
	BlockTimeMS int               `yaml:"block_time_ms"`
	Genesis     map[string]uint64 `yaml:"genesis"` // address -> funding (producer bootstrap)
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

func (c *Config) applyDefaults() {
	if c.DataDir == "" {
		c.DataDir = "./data"
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
	if c.BlockTimeMS == 0 {
		c.BlockTimeMS = 5000
	}
}
