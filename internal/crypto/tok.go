package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"golang.org/x/crypto/scrypt"
)

// .tok is ShadowLedger's encrypted wallet format. The Ed25519 private key is
// sealed with AES-256-GCM under a key derived from the user's passphrase via
// scrypt. This is what users should keep on disk for real funds — never the
// plaintext .json wallet.

// TokVersion is the current .tok format version.
const TokVersion = 1

type kdfParams struct {
	N    int    `json:"n"`
	R    int    `json:"r"`
	P    int    `json:"p"`
	Salt string `json:"salt"` // base64
}

type tokFile struct {
	Version    int       `json:"version"`
	Address    Address   `json:"address"`
	KDF        string    `json:"kdf"`
	KDFParams  kdfParams `json:"kdf_params"`
	Cipher     string    `json:"cipher"`
	Nonce      string    `json:"nonce"`      // base64
	Ciphertext string    `json:"ciphertext"` // base64
}

// scrypt cost parameters (interactive-grade; ~tens of ms).
const (
	scryptN = 1 << 15 // 32768
	scryptR = 8
	scryptP = 1
)

// Wallet errors.
var (
	ErrNoPassphrase = errors.New("crypto: .tok wallet requires a passphrase (set SL_WALLET_PASS or --pass)")
	ErrBadPass      = errors.New("crypto: wrong passphrase or corrupt .tok wallet")
)

func deriveKey(pass, salt []byte) ([]byte, error) {
	return scrypt.Key(pass, salt, scryptN, scryptR, scryptP, 32)
}

// SaveTok writes the key pair to path as an encrypted .tok wallet.
func (k *KeyPair) SaveTok(path, passphrase string) error {
	if passphrase == "" {
		return ErrNoPassphrase
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	dk, err := deriveKey([]byte(passphrase), salt)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(dk)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	ct := gcm.Seal(nil, nonce, k.Priv, []byte(k.Address())) // address as AAD
	tf := tokFile{
		Version:    TokVersion,
		Address:    k.Address(),
		KDF:        "scrypt",
		KDFParams:  kdfParams{N: scryptN, R: scryptR, P: scryptP, Salt: base64.StdEncoding.EncodeToString(salt)},
		Cipher:     "aes-256-gcm",
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ct),
	}
	b, err := json.MarshalIndent(tf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// LoadTok reads and decrypts a .tok wallet.
func LoadTok(path, passphrase string) (*KeyPair, error) {
	if passphrase == "" {
		return nil, ErrNoPassphrase
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tf tokFile
	if err := json.Unmarshal(b, &tf); err != nil {
		return nil, err
	}
	salt, err := base64.StdEncoding.DecodeString(tf.KDFParams.Salt)
	if err != nil {
		return nil, err
	}
	dk, err := scrypt.Key([]byte(passphrase), salt, tf.KDFParams.N, tf.KDFParams.R, tf.KDFParams.P, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(dk)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce, err := base64.StdEncoding.DecodeString(tf.Nonce)
	if err != nil {
		return nil, err
	}
	ct, err := base64.StdEncoding.DecodeString(tf.Ciphertext)
	if err != nil {
		return nil, err
	}
	priv, err := gcm.Open(nil, nonce, ct, []byte(tf.Address))
	if err != nil {
		return nil, ErrBadPass
	}
	if len(priv) != ed25519.PrivateKeySize {
		return nil, ErrBadPass
	}
	pk := ed25519.PrivateKey(priv)
	kp := &KeyPair{Priv: pk, Pub: pk.Public().(ed25519.PublicKey)}
	if kp.Address() != tf.Address {
		return nil, ErrBadPass
	}
	return kp, nil
}

// IsTok reports whether a path looks like an encrypted wallet (by extension).
func IsTok(path string) bool { return strings.HasSuffix(strings.ToLower(path), ".tok") }

// LoadWalletAuto loads either a plaintext .json or encrypted .tok wallet. The
// passphrase is required for .tok and ignored for .json.
func LoadWalletAuto(path, passphrase string) (*KeyPair, error) {
	if IsTok(path) {
		return LoadTok(path, passphrase)
	}
	return LoadWallet(path)
}

// SaveWalletAuto writes a .tok (encrypted, passphrase required) or .json wallet
// based on the path extension.
func (k *KeyPair) SaveWalletAuto(path, passphrase string) error {
	if IsTok(path) {
		return k.SaveTok(path, passphrase)
	}
	return k.SaveWallet(path)
}

// LoadOrCreateWalletAuto loads a wallet, creating and persisting a new one if
// absent, honoring the .tok/.json format implied by the path.
func LoadOrCreateWalletAuto(path, passphrase string) (*KeyPair, error) {
	if _, err := os.Stat(path); err == nil {
		return LoadWalletAuto(path, passphrase)
	}
	k, err := Generate()
	if err != nil {
		return nil, err
	}
	if err := k.SaveWalletAuto(path, passphrase); err != nil {
		return nil, err
	}
	return k, nil
}
