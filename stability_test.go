package main

import (
	"math"
	"os"
	"strings"
	"testing"
)

func TestContinueBlockchain_NonExistent(t *testing.T) {
	// Temporarily move the data directory if it exists
	if _, err := os.Stat(dbPath); err == nil {
		os.Rename(dbPath, dbPath+"_backup")
		defer os.Rename(dbPath+"_backup", dbPath)
	}

	_, err := ContinueBlockchain("")
	if err != ErrBlockchainNotFound {
		t.Fatalf("Expected ErrBlockchainNotFound, got %v", err)
	}
}

func TestNewUTXOTransaction_InvalidAddress(t *testing.T) {
	// Provide invalid destination address
	// It should fail in NewUTXOTransaction or NewTxOutput gracefully
	utxoSet := &UTXOSet{}
	_, err := NewUTXOTransaction("sender", "invalid_address", 1000, 10, "", utxoSet)
	if err == nil {
		t.Fatal("Expected error for invalid address, got nil")
	}
	// Verify it does not panic
}

// TestNewUTXOTransaction_InsufficientFunds verifies that calling NewUTXOTransaction
// with an amount that exceeds available UTXOs returns a clean "insufficient funds"
// error instead of calling os.Exit or panicking.
//
// Strategy: use an in-memory BadgerDB (no disk I/O, no hangs) with no UTXO
// entries, plus a temporary wallet.dat so CreateWallets() can resolve the
// sender address. FindSpendableOutputs will return 0 balance, triggering the
// "insufficient funds" path in NewUTXOTransaction.
func TestNewUTXOTransaction_InsufficientFunds(t *testing.T) {
	// Build an in-memory blockchain with no UTXOs — same helper as utxo_set_test.go
	chain := newTestBlockchain(t)
	utxoSet := &UTXOSet{Blockchain: chain}

	// Create a fresh sender wallet
	wallet, _, err := NewWallet()
	if err != nil {
		t.Fatalf("failed to create sender wallet: %v", err)
	}
	from := wallet.GetAddress()

	// Create a fresh recipient wallet
	toWallet, _, err := NewWallet()
	if err != nil {
		t.Fatalf("failed to create recipient wallet: %v", err)
	}
	to := toWallet.GetAddress()

	// Preserve any existing wallet.dat so we can restore it after the test.
	const tmpSuffix = ".stability_test_backup"
	if _, statErr := os.Stat(walletFile); statErr == nil {
		if renErr := os.Rename(walletFile, walletFile+tmpSuffix); renErr != nil {
			t.Fatalf("failed to back up wallet file: %v", renErr)
		}
		defer os.Rename(walletFile+tmpSuffix, walletFile) //nolint:errcheck
	}

	// Write a minimal wallet.dat containing only the sender wallet so that
	// CreateWallets() inside NewUTXOTransaction resolves the address.
	ws := &Wallets{Wallets: map[string]*Wallet{from: wallet}}
	if err := ws.SaveToFile(); err != nil {
		t.Fatalf("failed to write test wallet file: %v", err)
	}
	defer os.Remove(walletFile) //nolint:errcheck

	// The in-memory UTXO set is empty, so available balance is 0.
	// NewUTXOTransaction must return an "insufficient funds" error.
	_, txErr := NewUTXOTransaction(from, to, 1_000_000_000, 10_000_000, "", utxoSet)
	if txErr == nil {
		t.Fatal("Expected insufficient funds error, got nil")
	}
	if !strings.Contains(txErr.Error(), "insufficient funds") {
		t.Fatalf("Expected 'insufficient funds' in error, got: %v", txErr)
	}
}

func TestWallets_CorruptedWalletFile(t *testing.T) {
	// Write garbage to wallet.dat
	if _, err := os.Stat(walletFile); err == nil {
		os.Rename(walletFile, walletFile+"_backup")
		defer os.Rename(walletFile+"_backup", walletFile)
	}
	defer os.Remove(walletFile)

	os.WriteFile(walletFile, []byte("garbage data for wallet"), 0644)
	ws := &Wallets{}
	err := ws.LoadFromFile()
	if err == nil {
		t.Fatal("Expected error for corrupted wallet file, got nil")
	}
}

func TestIntToHex_NoPanic(t *testing.T) {
	values := []int64{math.MinInt64, -1, 0, 1, math.MaxInt64}
	for _, v := range values {
		res := IntToHex(v)
		if len(res) != 8 {
			t.Fatalf("Expected length 8 for IntToHex(%d), got %d", v, len(res))
		}
	}
}
