package storage

import (
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nicolocarcagni/sole/pkg/core"
	"github.com/nicolocarcagni/sole/pkg/wallet"
)

func TestContinueBlockchain_NonExistent(t *testing.T) {
	// Temporarily move the data directory if it exists
	if _, err := os.Stat(DbFile); err == nil {
		os.Rename(DbFile, DbFile+"_backup")
		defer os.Rename(DbFile+"_backup", DbFile)
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
// entries, plus a temporary wallet.dat so wallet.CreateWallets() can resolve the
// sender address. FindSpendableOutputs will return 0 balance, triggering the
// "insufficient funds" path in NewUTXOTransaction.
func TestNewUTXOTransaction_InsufficientFunds(t *testing.T) {
	// Build an in-memory blockchain with no UTXOs — same helper as utxo_set_test.go
	chain := newTestBlockchain(t)
	utxoSet := &UTXOSet{Blockchain: chain}

	// Create a fresh sender wallet
	senderWallet, _, err := wallet.NewWallet()
	if err != nil {
		t.Fatalf("failed to create sender wallet: %v", err)
	}
	from := senderWallet.GetAddress()

	// Create a fresh recipient wallet
	toWallet, _, err := wallet.NewWallet()
	if err != nil {
		t.Fatalf("failed to create recipient wallet: %v", err)
	}
	to := toWallet.GetAddress()

	// Preserve any existing wallet.dat so we can restore it after the test.
	const tmpSuffix = ".stability_test_backup"
	if _, statErr := os.Stat(wallet.WalletFile); statErr == nil {
		if renErr := os.Rename(wallet.WalletFile, wallet.WalletFile+tmpSuffix); renErr != nil {
			t.Fatalf("failed to back up wallet file: %v", renErr)
		}
		defer os.Rename(wallet.WalletFile+tmpSuffix, wallet.WalletFile) //nolint:errcheck
	}

	// Write a minimal wallet.dat containing only the sender wallet so that
	// wallet.CreateWallets() inside NewUTXOTransaction resolves the address.
	ws := &wallet.Wallets{Wallets: map[string]*wallet.Wallet{from: senderWallet}}
	if err := ws.SaveToFile(); err != nil {
		t.Fatalf("failed to write test wallet file: %v", err)
	}
	defer os.Remove(wallet.WalletFile) //nolint:errcheck

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
	if _, err := os.Stat(wallet.WalletFile); err == nil {
		os.Rename(wallet.WalletFile, wallet.WalletFile+"_backup")
		defer os.Rename(wallet.WalletFile+"_backup", wallet.WalletFile)
	}
	defer os.Remove(wallet.WalletFile)

	os.WriteFile(wallet.WalletFile, []byte("garbage data for wallet"), 0644)
	ws := &wallet.Wallets{}
	err := ws.LoadFromFile()
	if err == nil {
		t.Fatal("Expected error for corrupted wallet file, got nil")
	}
}

func TestIntToHex_NoPanic(t *testing.T) {
	values := []int64{math.MinInt64, -1, 0, 1, math.MaxInt64}
	for _, v := range values {
		res := core.IntToHex(v)
		if len(res) != 8 {
			t.Fatalf("Expected length 8 for core.IntToHex(%d), got %d", v, len(res))
		}
	}
}

func NewUTXOTransaction(from, to string, amount int64, fee int64, memo string, utxoSet *UTXOSet) (*core.Transaction, error) {
	var inputs []core.TxInput
	var outputs []core.TxOutput

	wallets, err := wallet.CreateWallets()
	if err != nil {
		return nil, fmt.Errorf("failed to create wallets: %w", err)
	}
	w := wallets.GetWalletRef(from)
	if w == nil {
		return nil, fmt.Errorf("invalid sender address %q: wallet not found", from)
	}
	pubKeyHash := core.HashPubKey(w.PublicKey)

	totalRequired := amount + fee

	acc, validOutputs, err := utxoSet.FindSpendableOutputs(pubKeyHash, totalRequired)
	if err != nil {
		return nil, fmt.Errorf("failed to find spendable outputs: %w", err)
	}

	if acc < totalRequired {
		return nil, fmt.Errorf("insufficient funds: available %d, required %d", acc, totalRequired)
	}

	for txid, outs := range validOutputs {
		txID, err := hex.DecodeString(txid)
		if err != nil {
			return nil, fmt.Errorf("failed to decode txid %s: %w", txid, err)
		}

		for _, out := range outs {
			input := core.TxInput{Txid: txID, Vout: out, Signature: nil, PubKey: w.PublicKey}
			inputs = append(inputs, input)
		}
	}

	if memo != "" {
		if len(memo) > 80 {
			memo = memo[:80]
		}
		outputs = append(outputs, core.TxOutput{Value: 0, PubKeyHash: []byte(memo)})
	}

	outDest, err := core.NewTxOutput(amount, to)
	if err != nil {
		return nil, fmt.Errorf("invalid destination address %q: %w", to, err)
	}
	outputs = append(outputs, *outDest)

	if acc > totalRequired {
		outChange, err := core.NewTxOutput(acc-totalRequired, from)
		if err != nil {
			return nil, fmt.Errorf("failed to create change output: %w", err)
		}
		outputs = append(outputs, *outChange)
	}

	tx := core.Transaction{ID: nil, Vin: inputs, Vout: outputs, Timestamp: time.Now().Unix()}
	privKey, err := w.GetPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve private key for %s: %w", from, err)
	}
	err = utxoSet.Blockchain.SignTransaction(&tx, privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return &tx, nil
}
