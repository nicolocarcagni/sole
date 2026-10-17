package p2p

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	"github.com/dgraph-io/badger/v3"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/nicolocarcagni/sole/pkg/consensus"
	"github.com/nicolocarcagni/sole/pkg/core"
	"github.com/nicolocarcagni/sole/pkg/storage"
)

func TestHandleBlockPropagatesUTXOError(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	validator := append(key.PublicKey.X.FillBytes(make([]byte, 32)), key.PublicKey.Y.FillBytes(make([]byte, 32))...)
	authorizedValidator := append([]byte{0x04}, validator...)
	oldValidators := append([]string(nil), consensus.AuthorizedValidators...)
	consensus.AuthorizedValidators = append(consensus.AuthorizedValidators, hex.EncodeToString(authorizedValidator))
	t.Cleanup(func() { consensus.AuthorizedValidators = oldValidators })

	dbPath, err := os.MkdirTemp(t.TempDir(), "blockchain-")
	if err != nil {
		t.Fatal(err)
	}
	db, err := badger.Open(badger.DefaultOptions(dbPath).WithLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	genesis, err := storage.NewGenesisBlock()
	if err != nil {
		t.Fatal(err)
	}
	genesisData, err := genesis.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(txn *badger.Txn) error {
		if err := txn.Set(genesis.Hash, genesisData); err != nil {
			return err
		}
		return txn.Set([]byte("lh"), genesis.Hash)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	readOnlyOptions := badger.DefaultOptions(dbPath).WithLogger(nil)
	readOnlyOptions.ReadOnly = true
	db, err = badger.Open(readOnlyOptions)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	chain := &storage.Blockchain{LastHash: append([]byte(nil), genesis.Hash...), Database: db}
	block := core.NewBlock(nil, genesis.Hash, genesis.Height+1, validator)
	consensus.MineBlock(block)
	if err := consensus.SignBlock(block, *key); err != nil {
		t.Fatal(err)
	}
	serializedBlock, err := block.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	var request bytes.Buffer
	if err := gob.NewEncoder(&request).Encode(BlockMsg{AddrFrom: "test", Block: serializedBlock}); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		Blockchain: chain,
		UTXOSet:    &storage.UTXOSet{Blockchain: chain},
		Mempool:    make(map[string]core.MempoolItem),
	}
	if err := server.HandleBlock(request.Bytes(), peer.ID("test-peer")); err == nil {
		t.Fatal("HandleBlock did not propagate the read-only database write error")
	}
}

func TestStartupRecoversInterruptedUTXOUpdate(t *testing.T) {
	dbPath := t.TempDir()
	db, err := badger.Open(badger.DefaultOptions(dbPath).WithLogger(nil))
	if err != nil {
		t.Fatal(err)
	}

	genesis := &core.Block{Hash: []byte("genesis"), Height: 0}
	tx := &core.Transaction{
		ID:   []byte("canonical-tx"),
		Vin:  []core.TxInput{{Txid: []byte{}, Vout: -1, PubKey: []byte("coinbase")}},
		Vout: []core.TxOutput{{Value: 10, PubKeyHash: []byte("canonical-owner")}},
	}
	block := &core.Block{
		Hash:          []byte("tip"),
		Height:        1,
		PrevBlockHash: genesis.Hash,
		Transactions:  []*core.Transaction{tx},
	}
	genesisData, err := genesis.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	blockData, err := block.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	staleKey := []byte(storage.UtxoPrefix + "stale-output-0")
	staleOutput, err := storage.SerializeUTXO(core.TxOutput{Value: 99, PubKeyHash: []byte("stale-owner")})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(txn *badger.Txn) error {
		if err := txn.Set(genesis.Hash, genesisData); err != nil {
			return err
		}
		if err := txn.Set(block.Hash, blockData); err != nil {
			return err
		}
		if err := txn.Set([]byte("lh"), block.Hash); err != nil {
			return err
		}
		return txn.Set(staleKey, staleOutput)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen after the simulated crash with a stale derived index.
	db, err = badger.Open(badger.DefaultOptions(dbPath).WithLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	chain := &storage.Blockchain{LastHash: append([]byte(nil), block.Hash...), Database: db}
	if err := recoverUTXOSet(&storage.UTXOSet{Blockchain: chain}); err != nil {
		t.Fatalf("startup recovery failed: %v", err)
	}

	canonicalKey := []byte(storage.UtxoPrefix + hex.EncodeToString(tx.ID) + "-0")
	if err := db.View(func(txn *badger.Txn) error {
		if _, err := txn.Get(canonicalKey); err != nil {
			return fmt.Errorf("canonical UTXO was not reconstructed: %w", err)
		}
		if _, err := txn.Get(staleKey); err != badger.ErrKeyNotFound {
			return fmt.Errorf("stale UTXO remains after recovery: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
