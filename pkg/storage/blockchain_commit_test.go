package storage

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/dgraph-io/badger/v3"

	"github.com/nicolocarcagni/sole/pkg/consensus"
)

func TestForgeBlockCommitFailureKeepsLastHash(t *testing.T) {
	genesis, err := NewGenesisBlock()
	if err != nil {
		t.Fatalf("NewGenesisBlock: %v", err)
	}
	chain := newTestBlockchain(t)
	writeBlockToDB(t, chain.Database, genesis)
	chain.LastHash = append([]byte(nil), genesis.Hash...)
	setCommitFailureAfterCallback(chain)
	lastHash := chain.GetLastHash()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if _, err := chain.ForgeBlock(nil, *key); err == nil {
		t.Fatal("ForgeBlock succeeded despite the simulated commit failure")
	}
	if got := chain.GetLastHash(); !bytes.Equal(got, lastHash) {
		t.Fatalf("LastHash changed after failed commit: got %x, want %x", got, lastHash)
	}
	assertDatabaseTip(t, chain, lastHash)
}

func TestAddBlockCommitFailureKeepsLastHash(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	validator := append([]byte{0x04}, key.PublicKey.X.FillBytes(make([]byte, 32))...)
	validator = append(validator, key.PublicKey.Y.FillBytes(make([]byte, 32))...)
	oldValidators := append([]string(nil), consensus.AuthorizedValidators...)
	consensus.AuthorizedValidators = append(consensus.AuthorizedValidators, hex.EncodeToString(validator))
	t.Cleanup(func() { consensus.AuthorizedValidators = oldValidators })

	genesis, err := NewGenesisBlock()
	if err != nil {
		t.Fatalf("NewGenesisBlock: %v", err)
	}
	chain := newTestBlockchain(t)
	writeBlockToDB(t, chain.Database, genesis)
	chain.LastHash = append([]byte(nil), genesis.Hash...)
	setCommitFailureAfterCallback(chain)
	lastHash := chain.GetLastHash()

	block := makeSignedCoinbaseBlock(t, key, genesis, "commit-failure", 1)

	if _, err := chain.AddBlock(block); err == nil {
		t.Fatal("AddBlock succeeded despite the simulated commit failure")
	}
	if got := chain.GetLastHash(); !bytes.Equal(got, lastHash) {
		t.Fatalf("LastHash changed after failed commit: got %x, want %x", got, lastHash)
	}
	assertDatabaseTip(t, chain, lastHash)
}

func TestGetLastHashReturnsCopy(t *testing.T) {
	chain := &Blockchain{LastHash: []byte{1, 2, 3}}
	lastHash := chain.GetLastHash()
	lastHash[0] = 9
	if got := chain.GetLastHash(); !bytes.Equal(got, []byte{1, 2, 3}) {
		t.Fatalf("GetLastHash returned a mutable reference: got %v", got)
	}
}

func setCommitFailureAfterCallback(chain *Blockchain) {
	chain.updateFn = func(update func(*badger.Txn) error) error {
		txn := chain.Database.NewTransaction(true)
		defer txn.Discard()
		if err := update(txn); err != nil {
			return err
		}
		return errors.New("simulated Badger commit failure")
	}
}

func assertDatabaseTip(t *testing.T, chain *Blockchain, expectedHash []byte) {
	t.Helper()
	var actualHash []byte
	if err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			return err
		}
		actualHash, err = item.ValueCopy(nil)
		return err
	}); err != nil {
		t.Fatalf("read persisted tip: %v", err)
	}
	if !bytes.Equal(actualHash, expectedHash) {
		t.Fatalf("persisted tip changed after failed commit: got %x, want %x", actualHash, expectedHash)
	}
}
