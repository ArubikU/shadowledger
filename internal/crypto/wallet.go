package crypto

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
)

// walletFile is the on-disk JSON representation of a key pair.
type walletFile struct {
	Priv    string  `json:"priv"`
	Pub     string  `json:"pub"`
	Address Address `json:"address"`
}

// SaveWallet writes the key pair to path as JSON.
func (k *KeyPair) SaveWallet(path string) error {
	wf := walletFile{
		Priv:    base64.StdEncoding.EncodeToString(k.Priv),
		Pub:     base64.StdEncoding.EncodeToString(k.Pub),
		Address: k.Address(),
	}
	b, err := json.MarshalIndent(wf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// LoadWallet reads a key pair from a JSON wallet file.
func LoadWallet(path string) (*KeyPair, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wf walletFile
	if err := json.Unmarshal(b, &wf); err != nil {
		return nil, err
	}
	priv, err := base64.StdEncoding.DecodeString(wf.Priv)
	if err != nil {
		return nil, err
	}
	pub, err := base64.StdEncoding.DecodeString(wf.Pub)
	if err != nil {
		return nil, err
	}
	if len(priv) != ed25519.PrivateKeySize || len(pub) != ed25519.PublicKeySize {
		return nil, ErrBadKeyLen
	}
	return &KeyPair{Priv: ed25519.PrivateKey(priv), Pub: ed25519.PublicKey(pub)}, nil
}

// LoadOrCreateWallet loads a wallet, creating and persisting a new one if absent.
func LoadOrCreateWallet(path string) (*KeyPair, error) {
	if _, err := os.Stat(path); err == nil {
		return LoadWallet(path)
	}
	k, err := Generate()
	if err != nil {
		return nil, err
	}
	if err := k.SaveWallet(path); err != nil {
		return nil, err
	}
	return k, nil
}
