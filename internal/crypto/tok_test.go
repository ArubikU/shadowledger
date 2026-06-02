package crypto

import (
	"path/filepath"
	"testing"
)

func TestTokRoundTrip(t *testing.T) {
	kp, _ := Generate()
	path := filepath.Join(t.TempDir(), "w.tok")
	if err := kp.SaveTok(path, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadTok(path, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if got.Address() != kp.Address() {
		t.Fatal("address mismatch after .tok round trip")
	}
	if string(got.Priv) != string(kp.Priv) {
		t.Fatal("private key mismatch after .tok round trip")
	}
}

func TestTokWrongPassphrase(t *testing.T) {
	kp, _ := Generate()
	path := filepath.Join(t.TempDir(), "w.tok")
	if err := kp.SaveTok(path, "right"); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTok(path, "wrong"); err != ErrBadPass {
		t.Fatalf("expected ErrBadPass, got %v", err)
	}
}

func TestTokRequiresPassphrase(t *testing.T) {
	kp, _ := Generate()
	path := filepath.Join(t.TempDir(), "w.tok")
	if err := kp.SaveTok(path, ""); err != ErrNoPassphrase {
		t.Fatalf("expected ErrNoPassphrase, got %v", err)
	}
}
