package storage

import (
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v3"

	"github.com/nicolocarcagni/sole/pkg/core"
	"github.com/nicolocarcagni/sole/pkg/consensus"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestBlockchain creates a minimal Blockchain backed by an in-memory
// BadgerDB instance. Using InMemory mode avoids file I/O, compaction
// goroutines, and the db.Close() hang that occurs when DropPrefix triggers
// a background flush in on-disk Badger.
func newTestBlockchain(t *testing.T) *Blockchain {
	t.Helper()
	opts := badger.DefaultOptions("").WithInMemory(true)
	opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("newTestBlockchain: failed to open in-memory BadgerDB: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("newTestBlockchain cleanup: db.Close() error: %v", err)
		}
	})
	return &Blockchain{LastHash: nil, Database: db, Mux: sync.Mutex{}}
}

// writeBlockToDB serializes a block and writes it to BadgerDB under its hash,
// also updating the "lh" (last-hash) key and the tx-<id> index.
// It bypasses ForgeBlock / AddBlock's signature validation — safe for unit tests.
func writeBlockToDB(t *testing.T, db *badger.DB, block *core.Block) {
	t.Helper()
	err := db.Update(func(txn *badger.Txn) error {
		serialized, err := block.Serialize()
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		if err := txn.Set(block.Hash, serialized); err != nil {
			return err
		}
		for _, tx := range block.Transactions {
			if err := txn.Set(append([]byte("tx-"), tx.ID...), block.Hash); err != nil {
				return err
			}
		}
		return txn.Set([]byte("lh"), block.Hash)
	})
	if err != nil {
		t.Fatalf("writeBlockToDB: %v", err)
	}
}

// makeMinedBlock builds a block and mines it (finds a valid PoW nonce).
// Validator and Signature are placeholder bytes; no PoA key needed.
func makeMinedBlock(t *testing.T, txs []*core.Transaction, prevHash []byte, height int) *core.Block {
	t.Helper()
	b := &core.Block{
		Timestamp:     time.Now().Unix(),
		Transactions:  txs,
		PrevBlockHash: prevHash,
		Hash:          []byte{},
		Height:        height,
		Nonce:         0,
		Validator:     []byte("test-validator"),
		Signature:     []byte{},
	}
	consensus.MineBlock(b)
	return b
}

// keyExists reports whether key k is present in BadgerDB.
func keyExists(t *testing.T, db *badger.DB, k string) bool {
	t.Helper()
	found := false
	err := db.View(func(txn *badger.Txn) error {
		_, err := txn.Get([]byte(k))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		return nil
	})
	if err != nil {
		t.Fatalf("keyExists: DB error for key %q: %v", k, err)
	}
	return found
}

// readUTXOValue fetches and deserializes a core.TxOutput stored at the given key.
func readUTXOValue(t *testing.T, db *badger.DB, k string) core.TxOutput {
	t.Helper()
	var out core.TxOutput
	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(k))
		if err != nil {
			return fmt.Errorf("key %q not found: %w", k, err)
		}
		v, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		out, err = DeserializeUTXO(v)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("readUTXOValue: %v", err)
	}
	return out
}

// ---------------------------------------------------------------------------
// Test 1 — Multi-Output Partial Spending & Reindex
// ---------------------------------------------------------------------------

// TestReindex_PreservesVoutIndex verifies that UTXOSet.Reindex() stores each
// unspent output under its ORIGINAL Vout index, not a compacted slice position.
//
// Setup:
//
//	TX1 has 3 outputs — indices 0 (Owner A), 1 (Owner B), 2 (Owner A).
//	TX2 spends TX1:0 and TX1:2 (both Owner A outputs).
//
// After Reindex():
//
//	utxo-<TX1>-0  must NOT exist (spent by TX2).
//	utxo-<TX1>-1  MUST exist with Value=200 locked to Owner B.
//	utxo-<TX1>-2  must NOT exist (spent by TX2).
//	FindSpendableOutputs(OwnerB) must return index 1 (never 0).
//	FindSpendableOutputs(OwnerA) must return no outputs from TX1.
func TestReindex_PreservesVoutIndex(t *testing.T) {
	chain := newTestBlockchain(t)

	// Owner key hashes (synthetic; not derived from real wallets).
	ownerAHash := []byte("owner-a-pubkey-hash-20byts")
	ownerBHash := []byte("owner-b-pubkey-hash-20byts")

	// Build TX1 with 3 outputs.
	tx1ID := []byte("test-tx1-id-0000000001")
	tx1 := &core.Transaction{
		ID:  tx1ID,
		Vin: []core.TxInput{{Txid: []byte{}, Vout: -1, Signature: nil, PubKey: []byte("coinbase")}},
		Vout: []core.TxOutput{
			{Value: 100, PubKeyHash: ownerAHash}, // Vout 0
			{Value: 200, PubKeyHash: ownerBHash}, // Vout 1
			{Value: 300, PubKeyHash: ownerAHash}, // Vout 2
		},
		Timestamp: time.Now().Unix(),
	}

	// core.Block 1: genesis-like (empty PrevBlockHash so iterator stops here).
	block1 := makeMinedBlock(t, []*core.Transaction{tx1}, []byte{}, 0)
	chain.LastHash = block1.Hash
	writeBlockToDB(t, chain.Database, block1)

	// Build TX2 spending TX1:0 and TX1:2.
	tx2ID := []byte("test-tx2-id-0000000002")
	tx2 := &core.Transaction{
		ID: tx2ID,
		Vin: []core.TxInput{
			{Txid: tx1ID, Vout: 0, Signature: []byte("sig-a0"), PubKey: []byte("pubkey-a")}, // spends TX1:0
			{Txid: tx1ID, Vout: 2, Signature: []byte("sig-a2"), PubKey: []byte("pubkey-a")}, // spends TX1:2
		},
		Vout: []core.TxOutput{
			{Value: 390, PubKeyHash: ownerAHash}, // change back to Owner A
		},
		Timestamp: time.Now().Unix(),
	}

	// core.Block 2: links back to core.Block 1.
	block2 := makeMinedBlock(t, []*core.Transaction{tx2}, block1.Hash, 1)
	chain.LastHash = block2.Hash
	writeBlockToDB(t, chain.Database, block2)

	// Run Reindex.
	utxoSet := UTXOSet{Blockchain: chain}
	utxoSet.Reindex()

	// --- Assertions: TX1 UTXO keys ---
	tx1IDHex := hex.EncodeToString(tx1ID)
	key0 := fmt.Sprintf("utxo-%s-0", tx1IDHex)
	key1 := fmt.Sprintf("utxo-%s-1", tx1IDHex)
	key2 := fmt.Sprintf("utxo-%s-2", tx1IDHex)

	// Vout 0 was spent — must not exist.
	if keyExists(t, chain.Database, key0) {
		t.Errorf("FAIL: key %q should not exist (TX1:0 was spent by TX2)", key0)
	}

	// Vout 1 was NOT spent — must exist with correct value and owner.
	if !keyExists(t, chain.Database, key1) {
		t.Fatalf("FAIL: key %q must exist (TX1:1 is unspent, Owner B)", key1)
	}
	out1 := readUTXOValue(t, chain.Database, key1)
	if out1.Value != 200 {
		t.Errorf("FAIL: TX1:1 value = %d, want 200", out1.Value)
	}
	if !out1.IsLockedWithKey(ownerBHash) {
		t.Errorf("FAIL: TX1:1 is not locked to Owner B (got PubKeyHash %x)", out1.PubKeyHash)
	}

	// Vout 2 was spent — must not exist.
	if keyExists(t, chain.Database, key2) {
		t.Errorf("FAIL: key %q should not exist (TX1:2 was spent by TX2)", key2)
	}

	// --- Assertions: FindSpendableOutputs for Owner B ---
	// Must return TX1 with index 1 — never index 0.
	_, spendableB, err := utxoSet.FindSpendableOutputs(ownerBHash, 1)
	if err != nil {
		t.Fatalf("FAIL: FindSpendableOutputs(OwnerB) returned error: %v", err)
	}
	tx1Indices, hasTX1 := spendableB[tx1IDHex]
	if !hasTX1 {
		t.Errorf("FAIL: FindSpendableOutputs(OwnerB) returned no outputs for TX1")
	} else if len(tx1Indices) != 1 || tx1Indices[0] != 1 {
		t.Errorf("FAIL: FindSpendableOutputs(OwnerB) returned TX1 indices %v, want [1]", tx1Indices)
	}

	// --- Assertions: FindSpendableOutputs for Owner A has no TX1 outputs ---
	// Owner A's only remaining UTXO is from TX2 (the change output), not TX1.
	_, spendableA, err2 := utxoSet.FindSpendableOutputs(ownerAHash, 1)
	if err2 != nil {
		t.Fatalf("FAIL: FindSpendableOutputs(OwnerA) returned error: %v", err2)
	}
	if indices, ok := spendableA[tx1IDHex]; ok {
		t.Errorf("FAIL: FindSpendableOutputs(OwnerA) returned TX1 indices %v; expected none (all TX1:OwnerA outputs were spent)", indices)
	}
}

// ---------------------------------------------------------------------------
// Test 2 — Reindex Consistency with Genesis
// ---------------------------------------------------------------------------

// TestReindex_GenesisConsistency verifies that running Reindex() immediately
// after genesis leaves the genesis coinbase output at index 0 intact and
// spendable, with no spurious index remapping.
func TestReindex_GenesisConsistency(t *testing.T) {
	chain := newTestBlockchain(t)

	// Build the genesis coinbase output (mirrors genesis.go exactly).
	pubKeyHash, err := core.ExtractPubKeyHash(core.GenesisAdminAddress)
	if err != nil {
		t.Fatalf("core.ExtractPubKeyHash(core.GenesisAdminAddress): %v", err)
	}

	txin := core.TxInput{Txid: []byte{}, Vout: -1, Signature: nil, PubKey: []byte(core.GenesisCoinbaseData)}
	txout, _ := core.NewTxOutput(int64(core.GenesisReward*100000000), core.GenesisAdminAddress)
	txout.PubKeyHash = pubKeyHash
	genesisTX := &core.Transaction{
		ID:        []byte("SOLE_GENESIS_TX_ID"),
		Vin:       []core.TxInput{txin},
		Vout:      []core.TxOutput{*txout},
		Timestamp: int64(core.GenesisTimestamp),
	}

	// Use time.Now() as the block timestamp so consensus.MineBlock doesn't hit a
	// historically-fixed timestamp that could affect nonce difficulty.
	genesisBlock := makeMinedBlock(t, []*core.Transaction{genesisTX}, []byte{}, 0)
	chain.LastHash = genesisBlock.Hash
	writeBlockToDB(t, chain.Database, genesisBlock)

	// Run Reindex.
	utxoSet := UTXOSet{Blockchain: chain}
	utxoSet.Reindex()

	// --- Assertions ---
	genesisTXIDHex := hex.EncodeToString(genesisTX.ID)
	key0 := fmt.Sprintf("utxo-%s-0", genesisTXIDHex)

	if !keyExists(t, chain.Database, key0) {
		t.Fatalf("FAIL: genesis UTXO key %q does not exist after Reindex()", key0)
	}

	out0 := readUTXOValue(t, chain.Database, key0)
	wantValue := int64(core.GenesisReward * 100000000)
	if out0.Value != wantValue {
		t.Errorf("FAIL: genesis output value = %d, want %d", out0.Value, wantValue)
	}
	if !out0.IsLockedWithKey(pubKeyHash) {
		t.Errorf("FAIL: genesis output is not locked to core.GenesisAdminAddress (PubKeyHash %x)", out0.PubKeyHash)
	}

	// FindSpendableOutputs must resolve genesis admin's output at index 0.
	_, spendable, errSpend := utxoSet.FindSpendableOutputs(pubKeyHash, 1)
	if errSpend != nil {
		t.Fatalf("FAIL: FindSpendableOutputs(GenesisAdmin) returned error: %v", errSpend)
	}
	indices, ok := spendable[genesisTXIDHex]
	if !ok {
		t.Errorf("FAIL: FindSpendableOutputs(GenesisAdmin) returned no outputs for genesis TX")
	} else if len(indices) != 1 || indices[0] != 0 {
		t.Errorf("FAIL: FindSpendableOutputs(GenesisAdmin) returned indices %v, want [0]", indices)
	}
}
