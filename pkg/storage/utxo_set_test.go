package storage

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v3"

	"github.com/nicolocarcagni/sole/pkg/consensus"
	"github.com/nicolocarcagni/sole/pkg/core"
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

func setupCanonicalUTXOTest(t *testing.T) (*Blockchain, *UTXOSet, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := append(key.PublicKey.X.FillBytes(make([]byte, 32)), key.PublicKey.Y.FillBytes(make([]byte, 32))...)
	validator := append([]byte{0x04}, pub...)
	oldValidators := append([]string(nil), consensus.AuthorizedValidators...)
	consensus.AuthorizedValidators = append(consensus.AuthorizedValidators, hex.EncodeToString(validator))
	t.Cleanup(func() { consensus.AuthorizedValidators = oldValidators })

	chain := newTestBlockchain(t)
	genesis, err := NewGenesisBlock()
	if err != nil {
		t.Fatal(err)
	}
	writeBlockToDB(t, chain.Database, genesis)
	chain.LastHash = append([]byte(nil), genesis.Hash...)
	utxo := &UTXOSet{Blockchain: chain}
	if err := utxo.Reindex(); err != nil {
		t.Fatal(err)
	}
	return chain, utxo, key
}

func makeSignedCoinbaseBlock(t *testing.T, key *ecdsa.PrivateKey, parent *core.Block, txID string, value int64) *core.Block {
	t.Helper()
	tx := &core.Transaction{
		ID:        []byte(txID),
		Vin:       []core.TxInput{{Txid: []byte{}, Vout: -1, PubKey: []byte("coinbase")}},
		Vout:      []core.TxOutput{{Value: value, PubKeyHash: []byte(txID)}},
		Timestamp: time.Now().Unix(),
	}
	block := &core.Block{
		Timestamp:     parent.Timestamp + 1,
		Transactions:  []*core.Transaction{tx},
		PrevBlockHash: append([]byte(nil), parent.Hash...),
		Height:        parent.Height + 1,
		Validator:     append(key.PublicKey.X.FillBytes(make([]byte, 32)), key.PublicKey.Y.FillBytes(make([]byte, 32))...),
	}
	consensus.MineBlock(block)
	if err := consensus.SignBlock(block, *key); err != nil {
		t.Fatalf("SignBlock: %v", err)
	}
	return block
}

func TestSideBranchDoesNotChangeUTXO(t *testing.T) {
	chain, utxo, key := setupCanonicalUTXOTest(t)
	genesis, err := chain.GetBlock(chain.LastHash)
	if err != nil {
		t.Fatal(err)
	}
	main := makeSignedCoinbaseBlock(t, key, &genesis, "main-branch-tx", 10)
	if added, err := chain.AddBlockWithUTXO(main, utxo); err != nil || !added {
		t.Fatal("failed to add canonical block")
	}
	side := makeSignedCoinbaseBlock(t, key, &genesis, "side-branch-tx", 20)
	if added, err := chain.AddBlockWithUTXO(side, utxo); err != nil || !added {
		t.Fatal("failed to store valid side-branch block")
	}

	if !keyExists(t, chain.Database, fmt.Sprintf("%s%s-0", UtxoPrefix, hex.EncodeToString([]byte("main-branch-tx")))) {
		t.Fatal("canonical branch UTXO disappeared")
	}
	if keyExists(t, chain.Database, fmt.Sprintf("%s%s-0", UtxoPrefix, hex.EncodeToString([]byte("side-branch-tx")))) {
		t.Fatal("side-branch UTXO was added")
	}
	if !bytes.Equal(chain.LastHash, main.Hash) {
		t.Fatal("side branch changed the canonical tip")
	}
}

func TestLongerForkRebuildsUTXOSet(t *testing.T) {
	chain, utxo, key := setupCanonicalUTXOTest(t)
	genesis, err := chain.GetBlock(chain.LastHash)
	if err != nil {
		t.Fatal(err)
	}
	main1 := makeSignedCoinbaseBlock(t, key, &genesis, "main-one", 10)
	main2 := makeSignedCoinbaseBlock(t, key, main1, "main-two", 11)
	for _, block := range []*core.Block{main1, main2} {
		if added, err := chain.AddBlockWithUTXO(block, utxo); err != nil || !added {
			t.Fatalf("failed to add canonical block %x", block.Hash)
		}
	}
	fork1 := makeSignedCoinbaseBlock(t, key, &genesis, "fork-one", 20)
	fork2 := makeSignedCoinbaseBlock(t, key, fork1, "fork-two", 21)
	fork3 := makeSignedCoinbaseBlock(t, key, fork2, "fork-three", 22)
	for _, block := range []*core.Block{fork1, fork2, fork3} {
		if added, err := chain.AddBlockWithUTXO(block, utxo); err != nil || !added {
			t.Fatalf("failed to add fork block %x", block.Hash)
		}
	}
	if !bytes.Equal(chain.LastHash, fork3.Hash) {
		t.Fatal("longer fork did not become canonical")
	}
	for _, id := range []string{"main-one", "main-two"} {
		key := fmt.Sprintf("%s%s-0", UtxoPrefix, hex.EncodeToString([]byte(id)))
		if keyExists(t, chain.Database, key) {
			t.Fatalf("orphaned UTXO %q remains after reorganization", id)
		}
	}
	for _, id := range []string{"fork-one", "fork-two", "fork-three"} {
		key := fmt.Sprintf("%s%s-0", UtxoPrefix, hex.EncodeToString([]byte(id)))
		if !keyExists(t, chain.Database, key) {
			t.Fatalf("canonical UTXO %q missing after reorganization", id)
		}
	}
}

func TestEqualHeightForkKeepsConsistentState(t *testing.T) {
	chain, utxo, key := setupCanonicalUTXOTest(t)
	genesis, err := chain.GetBlock(chain.LastHash)
	if err != nil {
		t.Fatal(err)
	}
	main := makeSignedCoinbaseBlock(t, key, &genesis, "equal-main", 10)
	if added, err := chain.AddBlockWithUTXO(main, utxo); err != nil || !added {
		t.Fatal("failed to add canonical block")
	}
	equal := makeSignedCoinbaseBlock(t, key, &genesis, "equal-fork", 12)
	if added, err := chain.AddBlockWithUTXO(equal, utxo); err != nil || !added {
		t.Fatal("failed to store equal-height fork")
	}
	if !bytes.Equal(chain.LastHash, main.Hash) {
		t.Fatal("equal-height fork changed the canonical tip")
	}
	if !keyExists(t, chain.Database, fmt.Sprintf("%s%s-0", UtxoPrefix, hex.EncodeToString([]byte("equal-main")))) {
		t.Fatal("canonical UTXO missing after equal-height fork")
	}
	if keyExists(t, chain.Database, fmt.Sprintf("%s%s-0", UtxoPrefix, hex.EncodeToString([]byte("equal-fork")))) {
		t.Fatal("equal-height fork UTXO polluted canonical state")
	}
}

func TestAddBlockAndUTXOAreAtomic(t *testing.T) {
	chain, utxo, key := setupCanonicalUTXOTest(t)
	parent, err := chain.GetBlock(chain.LastHash)
	if err != nil {
		t.Fatal(err)
	}
	transaction := &core.Transaction{
		ID:        bytes.Repeat([]byte{0xab}, 32500),
		Vin:       []core.TxInput{{Txid: []byte{}, Vout: -1, PubKey: []byte("coinbase")}},
		Vout:      []core.TxOutput{{Value: 1, PubKeyHash: []byte("atomic-owner")}},
		Timestamp: time.Now().Unix(),
	}
	validator := append(key.PublicKey.X.FillBytes(make([]byte, 32)), key.PublicKey.Y.FillBytes(make([]byte, 32))...)
	block := &core.Block{
		Timestamp:     parent.Timestamp + 1,
		Transactions:  []*core.Transaction{transaction},
		PrevBlockHash: append([]byte(nil), parent.Hash...),
		Height:        parent.Height + 1,
		Validator:     validator,
	}
	consensus.MineBlock(block)
	if err := consensus.SignBlock(block, *key); err != nil {
		t.Fatal(err)
	}
	initialUTXOCount, err := utxo.CountTransactions()
	if err != nil {
		t.Fatal(err)
	}

	added, err := chain.AddBlockWithUTXO(block, utxo)
	if err == nil {
		t.Fatal("expected oversized UTXO key to fail the Badger transaction")
	}
	if !strings.Contains(err.Error(), "failed to update canonical UTXO set") {
		t.Fatalf("block addition failed outside the UTXO write: %v", err)
	}
	if added {
		t.Fatal("block was reported added after its UTXO write failed")
	}
	if !bytes.Equal(chain.LastHash, parent.Hash) {
		t.Fatal("in-memory tip advanced after the atomic database transaction failed")
	}

	err = chain.Database.View(func(txn *badger.Txn) error {
		if _, err := txn.Get(block.Hash); err != badger.ErrKeyNotFound {
			return fmt.Errorf("failed block write was not rolled back: %v", err)
		}
		tip, err := txn.Get([]byte("lh"))
		if err != nil {
			return err
		}
		tipHash, err := tip.ValueCopy(nil)
		if err != nil {
			return err
		}
		if !bytes.Equal(tipHash, parent.Hash) {
			return fmt.Errorf("tip hash %x changed after the failed transaction", tipHash)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	finalUTXOCount, err := utxo.CountTransactions()
	if err != nil {
		t.Fatal(err)
	}
	if finalUTXOCount != initialUTXOCount {
		t.Fatalf("UTXO set changed after transaction failure: before %d, after %d", initialUTXOCount, finalUTXOCount)
	}
}

func TestStartupRecoversInterruptedUTXOUpdate(t *testing.T) {
	chain, utxo, key := setupCanonicalUTXOTest(t)
	parent, err := chain.GetBlock(chain.LastHash)
	if err != nil {
		t.Fatal(err)
	}
	block := makeSignedCoinbaseBlock(t, key, &parent, "recovery-block-tx", 1)

	// Simulate a block persisted before its derived UTXO index was updated.
	if added, err := chain.AddBlock(block); err != nil || !added {
		t.Fatal("failed to persist block for recovery scenario")
	}
	utxoKey := fmt.Sprintf("%s%s-0", UtxoPrefix, hex.EncodeToString(block.Transactions[0].ID))
	if keyExists(t, chain.Database, utxoKey) {
		t.Fatal("test setup unexpectedly created the block UTXO")
	}

	// Startup rebuilds the derived index from the canonical chain before serving.
	if err := utxo.Reindex(); err != nil {
		t.Fatalf("startup recovery failed: %v", err)
	}
	if !keyExists(t, chain.Database, utxoKey) {
		t.Fatal("startup recovery did not restore the canonical block UTXO")
	}
}

func TestReindexFailurePreservesPreviousUTXO(t *testing.T) {
	chain := newTestBlockchain(t)
	oldOutput, err := SerializeUTXO(core.TxOutput{Value: 7, PubKeyHash: []byte("previous")})
	if err != nil {
		t.Fatal(err)
	}
	if err := chain.Database.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(UtxoPrefix+"previous-0"), oldOutput)
	}); err != nil {
		t.Fatal(err)
	}
	chain.LastHash = []byte("missing-canonical-block")

	if err := (&UTXOSet{Blockchain: chain}).Reindex(); err == nil {
		t.Fatal("expected reindex to fail when the canonical tip is missing")
	}
	if !keyExists(t, chain.Database, UtxoPrefix+"previous-0") {
		t.Fatal("previous UTXO was removed after a failed reindex")
	}
}

func TestReindexLargeChainInBatches(t *testing.T) {
	chain := newTestBlockchain(t)
	count := int(chain.Database.MaxBatchCount()) + 1
	const transactionsPerBlock = 256
	var parentHash []byte
	for firstTx, height := 0, 1; firstTx < count; height++ {
		endTx := firstTx + transactionsPerBlock
		if endTx > count {
			endTx = count
		}
		block := &core.Block{
			Hash:          []byte(fmt.Sprintf("large-reindex-block-%04d", height)),
			PrevBlockHash: append([]byte(nil), parentHash...),
			Height:        height,
			Transactions:  make([]*core.Transaction, endTx-firstTx),
		}
		for i := firstTx; i < endTx; i++ {
			block.Transactions[i-firstTx] = &core.Transaction{
				ID:   []byte(fmt.Sprintf("large-tx-%08d", i)),
				Vout: []core.TxOutput{{Value: 1, PubKeyHash: []byte("large-owner")}},
			}
		}
		serialized, err := block.Serialize()
		if err != nil {
			t.Fatal(err)
		}
		if err := chain.Database.Update(func(txn *badger.Txn) error {
			return txn.Set(block.Hash, serialized)
		}); err != nil {
			t.Fatal(err)
		}
		parentHash = append(parentHash[:0], block.Hash...)
		firstTx = endTx
	}
	if err := chain.Database.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte("lh"), parentHash)
	}); err != nil {
		t.Fatal(err)
	}
	chain.LastHash = append([]byte(nil), parentHash...)

	utxo := &UTXOSet{Blockchain: chain}
	if err := utxo.Reindex(); err != nil {
		t.Fatalf("reindex failed for a chain larger than one Badger batch: %v", err)
	}
	got, err := utxo.CountTransactions()
	if err != nil {
		t.Fatal(err)
	}
	if got != count {
		t.Fatalf("reindexed UTXO count = %d, want %d", got, count)
	}
}

func TestReindexIsAtomic(t *testing.T) {
	chain := newTestBlockchain(t)
	oldOutput, err := SerializeUTXO(core.TxOutput{Value: 9, PubKeyHash: []byte("previous")})
	if err != nil {
		t.Fatal(err)
	}
	if err := chain.Database.Update(func(txn *badger.Txn) error {
		if err := txn.Set([]byte(UtxoPrefix+"previous-0"), oldOutput); err != nil {
			return err
		}
		block := &core.Block{
			Hash:   []byte("atomic-reindex-tip"),
			Height: 1,
			Transactions: []*core.Transaction{{
				ID:   bytes.Repeat([]byte{0xAB}, 65000),
				Vout: []core.TxOutput{{Value: 1, PubKeyHash: []byte("new-owner")}},
			}},
		}
		serialized, err := block.Serialize()
		if err != nil {
			return err
		}
		if err := txn.Set(block.Hash, serialized); err != nil {
			return err
		}
		return txn.Set([]byte("lh"), block.Hash)
	}); err != nil {
		t.Fatal(err)
	}
	chain.LastHash = []byte("atomic-reindex-tip")

	if err := (&UTXOSet{Blockchain: chain}).Reindex(); err == nil {
		t.Fatal("expected reindex to fail when the rebuilt UTXO key exceeds Badger's limit")
	}
	if !keyExists(t, chain.Database, UtxoPrefix+"previous-0") {
		t.Fatal("failed reindex exposed a partially replaced UTXO set")
	}
	if keyExists(t, chain.Database, UtxoPrefix+hex.EncodeToString(bytes.Repeat([]byte{0xAB}, 65000))+"-0") {
		t.Fatal("failed reindex exposed an invalid replacement UTXO")
	}
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
		if strings.HasPrefix(k, UtxoPrefix) {
			if item, err := txn.Get([]byte(activeUTXOPrefixKey)); err == nil {
				prefix, err := item.ValueCopy(nil)
				if err != nil {
					return err
				}
				k = string(prefix) + strings.TrimPrefix(k, UtxoPrefix)
			} else if err != badger.ErrKeyNotFound {
				return err
			}
		}
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
		if strings.HasPrefix(k, UtxoPrefix) {
			if item, err := txn.Get([]byte(activeUTXOPrefixKey)); err == nil {
				prefix, err := item.ValueCopy(nil)
				if err != nil {
					return err
				}
				k = string(prefix) + strings.TrimPrefix(k, UtxoPrefix)
			} else if err != badger.ErrKeyNotFound {
				return err
			}
		}
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
