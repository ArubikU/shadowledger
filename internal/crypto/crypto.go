// Package crypto provides ShadowLedger identity: Ed25519 keys, addresses, signing.
package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// AddressPrefix tags every ShadowLedger address.
const AddressPrefix = "sl"

// Address is a hex string: "sl" + hex(sha256(pubkey)[:20]).
type Address string

// AddressFromPub derives the canonical address from an Ed25519 public key.
func AddressFromPub(pub ed25519.PublicKey) Address {
	h := sha256.Sum256(pub)
	return Address(AddressPrefix + hex.EncodeToString(h[:20]))
}

// Valid reports whether a is a well-formed address.
func (a Address) Valid() bool {
	if len(a) != len(AddressPrefix)+40 {
		return false
	}
	if string(a[:2]) != AddressPrefix {
		return false
	}
	_, err := hex.DecodeString(string(a[2:]))
	return err == nil
}

// KeyPair holds an Ed25519 identity.
type KeyPair struct {
	Priv ed25519.PrivateKey
	Pub  ed25519.PublicKey
}

// Generate creates a fresh random key pair.
func Generate() (*KeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &KeyPair{Priv: priv, Pub: pub}, nil
}

// Address returns the key pair's address.
func (k *KeyPair) Address() Address { return AddressFromPub(k.Pub) }

// Sign signs msg with the private key.
func (k *KeyPair) Sign(msg []byte) []byte { return ed25519.Sign(k.Priv, msg) }

// Verify checks a signature against a public key and message.
func Verify(pub ed25519.PublicKey, msg, sig []byte) bool {
	if len(pub) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(pub, msg, sig)
}

// PubMatchesAddress reports whether pub hashes to addr.
func PubMatchesAddress(pub ed25519.PublicKey, addr Address) bool {
	return AddressFromPub(pub) == addr
}

// ErrBadKeyLen is returned when a key has the wrong size.
var ErrBadKeyLen = errors.New("crypto: bad key length")
