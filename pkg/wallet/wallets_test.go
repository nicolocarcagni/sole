package wallet

import (
	"bytes"
	"crypto/elliptic"
	"encoding/gob"
	"os"
	"testing"
)

func withWalletTestDir(t *testing.T) {
	t.Helper()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
}

func TestWalletFileEncryptsPrivateKey(t *testing.T) {
	withWalletTestDir(t)
	w, _, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	wallets := &Wallets{Wallets: map[string]*Wallet{w.GetAddress(): w}}
	if err := wallets.SaveToFile("correct horse battery staple"); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}
	data, err := os.ReadFile(WalletFile)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, w.PrivateKey) {
		t.Fatal("wallet file contains the private key in plaintext")
	}

	loaded := &Wallets{}
	if err := loaded.LoadFromFile("correct horse battery staple"); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if !bytes.Equal(loaded.Wallets[w.GetAddress()].PrivateKey, w.PrivateKey) {
		t.Fatal("wallet private key changed after encrypted round trip")
	}
}

func TestWalletLoadRejectsWrongPassword(t *testing.T) {
	withWalletTestDir(t)
	w, _, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	wallets := &Wallets{Wallets: map[string]*Wallet{w.GetAddress(): w}}
	if err := wallets.SaveToFile("correct password"); err != nil {
		t.Fatal(err)
	}
	if err := (&Wallets{}).LoadFromFile("wrong password"); err == nil {
		t.Fatal("LoadFromFile accepted a wrong password")
	}
}

func TestWalletSaveEnforcesFilePermissions(t *testing.T) {
	withWalletTestDir(t)
	w, _, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(WalletFile, []byte("existing file"), 0644); err != nil {
		t.Fatal(err)
	}
	wallets := &Wallets{Wallets: map[string]*Wallet{w.GetAddress(): w}}
	if err := wallets.SaveToFile("correct password"); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}
	info, err := os.Stat(WalletFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("wallet file permissions = %04o, want 0600", got)
	}
}

func TestMigrateLegacyWalletFile(t *testing.T) {
	withWalletTestDir(t)
	w, _, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	legacy := &Wallets{Wallets: map[string]*Wallet{w.GetAddress(): w}}
	var data bytes.Buffer
	gob.Register(elliptic.P256())
	if err := gob.NewEncoder(&data).Encode(legacy); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(WalletFile, data.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	if err := (&Wallets{}).MigrateLegacyWalletFile("new wallet password"); err != nil {
		t.Fatalf("MigrateLegacyWalletFile: %v", err)
	}
	contents, err := os.ReadFile(WalletFile)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(contents, w.PrivateKey) {
		t.Fatal("migrated wallet file contains the private key in plaintext")
	}
	loaded := &Wallets{}
	if err := loaded.LoadFromFile("new wallet password"); err != nil {
		t.Fatalf("LoadFromFile after migration: %v", err)
	}
	if !bytes.Equal(loaded.Wallets[w.GetAddress()].PrivateKey, w.PrivateKey) {
		t.Fatal("private key changed during migration")
	}
}
