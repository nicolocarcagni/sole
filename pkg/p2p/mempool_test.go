package p2p

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"github.com/dgraph-io/badger/v3"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/nicolocarcagni/sole/pkg/core"
	"github.com/nicolocarcagni/sole/pkg/storage"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// makeMempoolServer returns a minimal Server wired to the provided blockchain
// and UTXO set. Host is nil — relay is only reached after successful admission,
// so nil is safe for rejection-path tests.
func makeMempoolServer(t *testing.T, chain *storage.Blockchain, utxo *storage.UTXOSet) *Server {
	t.Helper()

	return &Server{
		Host:       nil,
		Blockchain: chain,
		UTXOSet:    utxo,
		Mempool:    make(map[string]core.MempoolItem),
		KnownPeers: make(map[string]string),
	}
}

// encodeTxMsg gob-encodes a TxMsg (the wire format expected by HandleTx).
func encodeTxMsg(t *testing.T, tx *core.Transaction) []byte {
	t.Helper()
	msg := TxMsg{AddrFrom: "test", Transaction: tx.Serialize()}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(msg); err != nil {
		t.Fatalf("encodeTxMsg: %v", err)
	}
	return buf.Bytes()
}

// suppress the "declared and not used" error for the sync import
var _ sync.Mutex

// ---------------------------------------------------------------------------
// Test 1 — Fake / Invalid Signature Rejection
// ---------------------------------------------------------------------------

// TestHandleTx_RejectsFakeSignature creates a transaction that references a
// valid UTXO but is signed with the wrong private key. It verifies that
// HandleTx rejects the transaction and the mempool stays empty.
func TestHandleTx_RejectsFakeSignature(t *testing.T) {
	chain := newTestBlockchain(t)
	utxo := &storage.UTXOSet{Blockchain: chain}
	srv := makeMempoolServer(t, chain, utxo)

	// Legitimate owner key pair
	ownerPriv, ownerPubKey := generateTestKeyPair()
	ownerPubKeyHash := core.HashPubKey(ownerPubKey)

	// Attacker key pair — different from the legitimate owner
	_, attackerPubKey := generateTestKeyPair()

	// Build a parent transaction whose output is owned by the legitimate owner.
	parentTxID := []byte("mempool-test-parent-txid-000001")
	parentTx := &core.Transaction{
		ID:  parentTxID,
		Vin: []core.TxInput{{Txid: []byte{}, Vout: -1, PubKey: []byte("coinbase")}},
		Vout: []core.TxOutput{
			{Value: 1000, PubKeyHash: ownerPubKeyHash},
		},
		Timestamp: time.Now().Unix(),
	}

	// Persist the parent UTXO so CalculateFee finds it.
	parentKey := fmt.Sprintf("%s%s-%d", storage.UtxoPrefix, hex.EncodeToString(parentTxID), 0)
	chain.Database.Update(func(txn *badger.Txn) error {

		serialized, err := storage.SerializeUTXO(parentTx.Vout[0])
		if err != nil {
			return err
		}
		return txn.Set([]byte(parentKey), serialized)
	})
	// Index the parent tx so VerifyTransactionWithMempool can look it up.
	genesisBlock := makeMinedBlock(t, []*core.Transaction{parentTx}, []byte{}, 0)
	chain.LastHash = genesisBlock.Hash
	writeBlockToDB(t, chain.Database, genesisBlock)

	// Build the spending transaction with the attacker's PubKey in Vin.
	spendTx := core.Transaction{
		Vin: []core.TxInput{
			{Txid: parentTxID, Vout: 0, PubKey: attackerPubKey},
		},
		Vout: []core.TxOutput{
			{Value: 900, PubKeyHash: core.HashPubKey(attackerPubKey)},
		},
		Timestamp: time.Now().Unix(),
	}
	// Sign with the legitimate owner's key, but the PubKey in Vin is the
	// attacker's — the ownership (PubKey hash) won't match the output's
	// PubKeyHash, so verification must fail.
	prevTXs := map[string]core.Transaction{hex.EncodeToString(parentTxID): *parentTx}
	spendTx.Sign(ownerPriv, prevTXs)
	// Override PubKey with attacker's key to trigger the mismatch.
	spendTx.Vin[0].PubKey = attackerPubKey
	spendTx.ID = spendTx.Hash()

	srv.HandleTx(encodeTxMsg(t, &spendTx), peer.ID("attacker-peer"))

	srv.MempoolMux.Lock()
	mLen := len(srv.Mempool)
	srv.MempoolMux.Unlock()

	if mLen != 0 {
		t.Errorf("FAIL: mempool should be empty after fake-signature rejection, got %d entries", mLen)
	}
}

// ---------------------------------------------------------------------------
// Test 2 — Historical Spent Output (Double-Spend) Rejection
// ---------------------------------------------------------------------------

// TestCalculateFee_RejectsSpentOutput verifies that once an output has been
// spent (its UTXO key removed by storage.UTXOSet.Update), CalculateFee returns an
// error containing "already been spent" instead of a positive fee.
func TestCalculateFee_RejectsSpentOutput(t *testing.T) {
	chain := newTestBlockchain(t)
	utxo := &storage.UTXOSet{Blockchain: chain}

	ownerHash := []byte("owner-pubkey-hash-20bytesXXXXXXXX")[:20]

	// core.Block 1: coinbase transaction
	coinbaseTxID := []byte("cb-tx-id-000000000001234567890")
	coinbaseTx := &core.Transaction{
		ID:  coinbaseTxID,
		Vin: []core.TxInput{{Txid: []byte{}, Vout: -1, PubKey: []byte("coinbase")}},
		Vout: []core.TxOutput{
			{Value: 5000, PubKeyHash: ownerHash},
		},
		Timestamp: time.Now().Unix(),
	}
	block1 := makeMinedBlock(t, []*core.Transaction{coinbaseTx}, []byte{}, 0)
	chain.LastHash = block1.Hash
	writeBlockToDB(t, chain.Database, block1)
	utxo.Reindex()

	// Sanity: coinbase UTXO should exist after Reindex.
	coinbaseID := hex.EncodeToString(coinbaseTxID)
	_, outputs, err := utxo.FindSpendableOutputs(ownerHash, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs[coinbaseID]) != 1 {
		t.Fatalf("setup: coinbase UTXO %q not found after Reindex", coinbaseID)
	}

	// core.Block 2: spend the coinbase output
	spendingTx := &core.Transaction{
		ID: []byte("spending-tx-id-0000000000000002"),
		Vin: []core.TxInput{
			{Txid: coinbaseTxID, Vout: 0, Signature: []byte("sig"), PubKey: []byte("pubkey")},
		},
		Vout: []core.TxOutput{
			{Value: 4000, PubKeyHash: ownerHash},
		},
		Timestamp: time.Now().Unix(),
	}
	block2 := makeMinedBlock(t, []*core.Transaction{spendingTx}, block1.Hash, 1)
	chain.LastHash = block2.Hash
	writeBlockToDB(t, chain.Database, block2)
	utxo.Update(block2)

	// Sanity: UTXO key must be gone after Update.
	_, outputs, err = utxo.FindSpendableOutputs(ownerHash, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs[coinbaseID]) != 0 {
		t.Fatalf("setup: coinbase UTXO %q should be deleted after block2", coinbaseID)
	}

	// Craft a double-spend transaction referencing the already-spent output.
	doubleSpendTx := &core.Transaction{
		ID: []byte("double-spend-tx-id-000000000003"),
		Vin: []core.TxInput{
			{Txid: coinbaseTxID, Vout: 0, Signature: []byte("sig2"), PubKey: []byte("pubkey2")},
		},
		Vout: []core.TxOutput{
			{Value: 4000, PubKeyHash: ownerHash},
		},
		Timestamp: time.Now().Unix(),
	}

	_, err = utxo.CalculateFee(doubleSpendTx, map[string]core.MempoolItem{})
	if err == nil {
		t.Fatalf("FAIL: CalculateFee should return an error for a spent output, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("already been spent")) {
		t.Errorf("FAIL: expected error to contain 'already been spent', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test 3 — Coinbase P2P Admission Rejection
// ---------------------------------------------------------------------------

// TestHandleTx_RejectsCoinbase verifies that a coinbase transaction submitted
// via HandleTx is rejected immediately and never enters the mempool.
func TestHandleTx_RejectsCoinbase(t *testing.T) {
	chain := newTestBlockchain(t)
	utxo := &storage.UTXOSet{Blockchain: chain}
	srv := makeMempoolServer(t, chain, utxo)

	// Build a minimal valid blockchain (empty genesis) so FindTransaction works.
	ownerHash := []byte("owner-pubkey-hash-20bytesXXXXXXXX")[:20]
	genesisTx := &core.Transaction{
		ID:  []byte("coinbase-test-genesis-id-000001"),
		Vin: []core.TxInput{{Txid: []byte{}, Vout: -1, PubKey: []byte("genesis-coinbase")}},
		Vout: []core.TxOutput{
			{Value: 5000000000, PubKeyHash: ownerHash},
		},
		Timestamp: time.Now().Unix(),
	}
	genesisBlock := makeMinedBlock(t, []*core.Transaction{genesisTx}, []byte{}, 0)
	chain.LastHash = genesisBlock.Hash
	writeBlockToDB(t, chain.Database, genesisBlock)

	// Craft a standalone coinbase transaction directly (avoids base58 address parsing).
	coinbaseTx := &core.Transaction{
		ID:  []byte("standalone-coinbase-tx-id-000001"),
		Vin: []core.TxInput{{Txid: []byte{}, Vout: -1, PubKey: []byte("miner-reward")}},
		Vout: []core.TxOutput{
			{Value: 1000, PubKeyHash: ownerHash},
		},
		Timestamp: time.Now().Unix(),
	}
	coinbaseTx.ID = coinbaseTx.Hash()

	srv.HandleTx(encodeTxMsg(t, coinbaseTx), peer.ID("fake-peer"))

	srv.MempoolMux.Lock()
	mLen := len(srv.Mempool)
	srv.MempoolMux.Unlock()

	if mLen != 0 {
		t.Errorf("FAIL: mempool should be empty after coinbase rejection, got %d entries", mLen)
	}
}

// ---------------------------------------------------------------------------
// Test 4 — Selective Mempool Eviction After Mining
// ---------------------------------------------------------------------------

// TestAttemptMine_SelectiveEviction adds TX_A and TX_B to the mempool, then
// simulates the selective-eviction loop from AttemptMine with a block that
// only includes TX_A. TX_A must be removed; TX_B must remain.
func TestAttemptMine_SelectiveEviction(t *testing.T) {
	chain := newTestBlockchain(t)
	utxo := &storage.UTXOSet{Blockchain: chain}
	srv := makeMempoolServer(t, chain, utxo)

	txA := &core.Transaction{
		ID:        []byte("tx-a-id-00000000000000000000001"),
		Vin:       []core.TxInput{{Txid: []byte("parent-a"), Vout: 0, Signature: []byte("sig"), PubKey: []byte("pk")}},
		Vout:      []core.TxOutput{{Value: 100, PubKeyHash: []byte("owner")}},
		Timestamp: time.Now().Unix(),
	}
	txB := &core.Transaction{
		ID:        []byte("tx-b-id-00000000000000000000002"),
		Vin:       []core.TxInput{{Txid: []byte("parent-b"), Vout: 0, Signature: []byte("sig"), PubKey: []byte("pk")}},
		Vout:      []core.TxOutput{{Value: 200, PubKeyHash: []byte("owner")}},
		Timestamp: time.Now().Unix(),
	}

	txAID := hex.EncodeToString(txA.ID)
	txBID := hex.EncodeToString(txB.ID)

	srv.MempoolMux.Lock()
	srv.Mempool[txAID] = core.MempoolItem{Tx: *txA, AddedAt: time.Now().Unix()}
	srv.Mempool[txBID] = core.MempoolItem{Tx: *txB, AddedAt: time.Now().Unix()}
	srv.MempoolMux.Unlock()

	// Simulate the selective-eviction loop: coinbase + TX_A only.
	// Build coinbase directly to avoid base58 address parsing in core.NewCoinbaseTX.
	coinbaseTx := &core.Transaction{
		ID:  []byte("mined-coinbase-tx-id-00000001"),
		Vin: []core.TxInput{{Txid: []byte{}, Vout: -1, PubKey: []byte("miner-reward")}},
		Vout: []core.TxOutput{
			{Value: 1000, PubKeyHash: []byte("miner-pk-hash")},
		},
		Timestamp: time.Now().Unix(),
	}
	coinbaseTx.ID = coinbaseTx.Hash()
	minedBlock := &core.Block{
		Transactions: []*core.Transaction{coinbaseTx, txA},
	}

	srv.MempoolMux.Lock()
	for _, tx := range minedBlock.Transactions {
		if !tx.IsCoinbase() {
			delete(srv.Mempool, hex.EncodeToString(tx.ID))
		}
	}
	srv.MempoolMux.Unlock()

	srv.MempoolMux.Lock()
	_, txAStill := srv.Mempool[txAID]
	_, txBStill := srv.Mempool[txBID]
	srv.MempoolMux.Unlock()

	if txAStill {
		t.Errorf("FAIL: TX_A should have been evicted from the mempool after mining")
	}
	if !txBStill {
		t.Errorf("FAIL: TX_B should still be in the mempool (it was not mined)")
	}
}

// ---------------------------------------------------------------------------
// Additional — HandleTx Rejects Already-Confirmed core.Transaction
// ---------------------------------------------------------------------------

// TestHandleTx_RejectsConfirmedTx verifies that a transaction already indexed
// in the blockchain is silently dropped by HandleTx without entering the mempool.
func TestHandleTx_RejectsConfirmedTx(t *testing.T) {
	chain := newTestBlockchain(t)
	utxo := &storage.UTXOSet{Blockchain: chain}
	srv := makeMempoolServer(t, chain, utxo)

	ownerHash := []byte("owner-pubkey-hash-20bytesXXXXXXXX")[:20]
	confirmedTx := &core.Transaction{
		ID:  []byte("already-confirmed-txid-0000001"),
		Vin: []core.TxInput{{Txid: []byte{}, Vout: -1, PubKey: []byte("coinbase")}},
		Vout: []core.TxOutput{
			{Value: 1000, PubKeyHash: ownerHash},
		},
		Timestamp: time.Now().Unix(),
	}
	block := makeMinedBlock(t, []*core.Transaction{confirmedTx}, []byte{}, 0)
	chain.LastHash = block.Hash
	writeBlockToDB(t, chain.Database, block)

	srv.HandleTx(encodeTxMsg(t, confirmedTx), peer.ID("fake-peer"))

	srv.MempoolMux.Lock()
	mLen := len(srv.Mempool)
	srv.MempoolMux.Unlock()

	if mLen != 0 {
		t.Errorf("FAIL: already-confirmed transaction should be rejected, mempool has %d entries", mLen)
	}
}

// ---------------------------------------------------------------------------
// Additional — MaxMempoolSize capacity enforcement
// ---------------------------------------------------------------------------

// TestHandleTx_RejectsWhenMempoolFull verifies that HandleTx stops admitting
// new transactions once the mempool reaches MaxMempoolSize entries.
func TestHandleTx_RejectsWhenMempoolFull(t *testing.T) {
	chain := newTestBlockchain(t)
	utxo := &storage.UTXOSet{Blockchain: chain}
	srv := makeMempoolServer(t, chain, utxo)

	// Pre-fill the mempool to the limit with stub entries.
	srv.MempoolMux.Lock()
	for i := 0; i < MaxMempoolSize; i++ {
		id := fmt.Sprintf("stub-mempool-tx-%08d", i)
		srv.Mempool[id] = core.MempoolItem{Tx: core.Transaction{ID: []byte(id)}, AddedAt: time.Now().Unix()}
	}
	srv.MempoolMux.Unlock()

	// Build a transaction that would otherwise be valid.
	ownerHash := []byte("owner-pk-hash-20bXXXXXXXXXXXXXXX")[:20]
	parentTxID := []byte("cap-test-parent-txid-000000001")
	parentTx := &core.Transaction{
		ID:  parentTxID,
		Vin: []core.TxInput{{Txid: []byte{}, Vout: -1, PubKey: []byte("coinbase")}},
		Vout: []core.TxOutput{
			{Value: 9999, PubKeyHash: ownerHash},
		},
		Timestamp: time.Now().Unix(),
	}
	parentKey := fmt.Sprintf("%s%s-%d", storage.UtxoPrefix, hex.EncodeToString(parentTxID), 0)
	chain.Database.Update(func(txn *badger.Txn) error {

		serialized, err := storage.SerializeUTXO(parentTx.Vout[0])
		if err != nil {
			return err
		}
		return txn.Set([]byte(parentKey), serialized)
	})
	genesisBlock := makeMinedBlock(t, []*core.Transaction{parentTx}, []byte{}, 0)
	chain.LastHash = genesisBlock.Hash
	writeBlockToDB(t, chain.Database, genesisBlock)

	privKey, pubKey := generateTestKeyPair()
	spendTx := core.Transaction{
		Vin:       []core.TxInput{{Txid: parentTxID, Vout: 0, PubKey: pubKey}},
		Vout:      []core.TxOutput{{Value: 100, PubKeyHash: core.HashPubKey(pubKey)}},
		Timestamp: time.Now().Unix(),
	}
	prevTXs := map[string]core.Transaction{hex.EncodeToString(parentTxID): *parentTx}
	spendTx.Sign(privKey, prevTXs)

	srv.MempoolMux.Lock()
	beforeLen := len(srv.Mempool)
	srv.MempoolMux.Unlock()

	srv.HandleTx(encodeTxMsg(t, &spendTx), peer.ID("fake-peer"))

	srv.MempoolMux.Lock()
	afterLen := len(srv.Mempool)
	srv.MempoolMux.Unlock()

	if afterLen != beforeLen {
		t.Errorf("FAIL: mempool size changed from %d to %d; should have been rejected due to capacity limit",
			beforeLen, afterLen)
	}
}

// Helpers copied from storage tests

func generateTestKeyPair() (ecdsa.PrivateKey, []byte) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pubKey := elliptic.Marshal(elliptic.P256(), priv.PublicKey.X, priv.PublicKey.Y)
	return *priv, pubKey
}

func newTestBlockchain(t *testing.T) *storage.Blockchain {
	t.Helper()
	opts := badger.DefaultOptions("").WithInMemory(true)
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("failed to open in-memory DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	genesis := &core.Block{
		Transactions:  []*core.Transaction{},
		PrevBlockHash: []byte{},
		Hash:          []byte("genesis"),
		Height:        0,
	}
	writeBlockToDB(t, db, genesis)

	return &storage.Blockchain{LastHash: genesis.Hash, Database: db}
}

func writeBlockToDB(t *testing.T, db *badger.DB, block *core.Block) {
	t.Helper()
	err := db.Update(func(txn *badger.Txn) error {
		serialized, _ := block.Serialize()
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
		t.Fatalf("failed to write block to db: %v", err)
	}
}

func makeMinedBlock(t *testing.T, txs []*core.Transaction, prevHash []byte, height int) *core.Block {
	t.Helper()
	block := core.NewBlock(txs, prevHash, height, []byte("validator"))
	return block
}

func createTestSpendTx(t *testing.T, chain *storage.Blockchain, parentSuffix string, val int64, fee int64) *core.Transaction {
	t.Helper()
	privKey, pubKey := generateTestKeyPair()
	pubKeyHash := core.HashPubKey(pubKey)

	parentTxID := []byte("parent-txid-" + parentSuffix)
	parentTx := &core.Transaction{
		ID:  parentTxID,
		Vin: []core.TxInput{{Txid: []byte{}, Vout: -1, PubKey: []byte("coinbase")}},
		Vout: []core.TxOutput{
			{Value: val + fee, PubKeyHash: pubKeyHash},
		},
		Timestamp: time.Now().Unix(),
	}
	parentKey := fmt.Sprintf("%s%s-%d", storage.UtxoPrefix, hex.EncodeToString(parentTxID), 0)
	chain.Database.Update(func(txn *badger.Txn) error {
		serialized, err := storage.SerializeUTXO(parentTx.Vout[0])
		if err != nil {
			return err
		}
		return txn.Set([]byte(parentKey), serialized)
	})
	block := makeMinedBlock(t, []*core.Transaction{parentTx}, chain.LastHash, 1)
	chain.LastHash = block.Hash
	writeBlockToDB(t, chain.Database, block)

	spendTx := core.Transaction{
		Vin:       []core.TxInput{{Txid: parentTxID, Vout: 0, PubKey: pubKey}},
		Vout:      []core.TxOutput{{Value: val, PubKeyHash: pubKeyHash}},
		Timestamp: time.Now().Unix(),
	}
	prevTXs := map[string]core.Transaction{hex.EncodeToString(parentTxID): *parentTx}
	spendTx.Sign(privKey, prevTXs)
	return &spendTx
}

// ---------------------------------------------------------------------------
// [P1-03] Mempool Hardening & Byte Budget Tests
// ---------------------------------------------------------------------------

// TestMempoolRejectsOversizedTransaction verifies that:
//  1. Raw wire messages exceeding MaxTxMsgSize are rejected immediately before
//     allocating a gob decoder.
//  2. Transaction payloads exceeding core.MaxTxSize are rejected before expensive
//     deserialization or signature checks.
func TestMempoolRejectsOversizedTransaction(t *testing.T) {
	chain := newTestBlockchain(t)
	utxo := &storage.UTXOSet{Blockchain: chain}
	srv := makeMempoolServer(t, chain, utxo)

	// 1. Raw wire payload exceeding MaxTxMsgSize
	oversizedWire := make([]byte, MaxTxMsgSize+1)
	srv.HandleTx(oversizedWire, peer.ID("peer-oversized-wire"))

	srv.MempoolMux.Lock()
	mLen1 := len(srv.Mempool)
	srv.MempoolMux.Unlock()
	if mLen1 != 0 {
		t.Fatalf("FAIL: oversized wire payload (%d bytes) should be rejected before decoding, mempool has %d items",
			len(oversizedWire), mLen1)
	}

	// 2. Structurally valid gob message with oversized Transaction payload (> core.MaxTxSize)
	oversizedTxData := make([]byte, core.MaxTxSize+1)
	msg := TxMsg{
		AddrFrom:    "peer-oversized",
		Transaction: oversizedTxData,
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(msg); err != nil {
		t.Fatalf("failed to encode test message: %v", err)
	}

	srv.HandleTx(buf.Bytes(), peer.ID("peer-oversized-tx"))

	srv.MempoolMux.Lock()
	mLen2 := len(srv.Mempool)
	srv.MempoolMux.Unlock()
	if mLen2 != 0 {
		t.Fatalf("FAIL: oversized transaction payload (%d bytes) should be rejected, mempool has %d items",
			len(oversizedTxData), mLen2)
	}
}

// TestMempoolEnforcesByteBudget verifies that transactions are rejected once the
// mempool reaches its aggregate byte budget, even if the item count is below MaxMempoolSize.
func TestMempoolEnforcesByteBudget(t *testing.T) {
	chain := newTestBlockchain(t)
	utxo := &storage.UTXOSet{Blockchain: chain}
	srv := makeMempoolServer(t, chain, utxo)

	tx1 := createTestSpendTx(t, chain, "budget-1", 1000, 10)
	tx1Bytes := encodeTxMsg(t, tx1)

	// Configure mempool budget to accommodate tx1 but reject subsequent transactions
	srv.MaxMempoolBytes = len(tx1.Serialize()) + 10

	// tx1 should be admitted
	srv.HandleTx(tx1Bytes, peer.ID("peer-1"))

	srv.MempoolMux.Lock()
	lenAfter1 := len(srv.Mempool)
	srv.MempoolMux.Unlock()
	if lenAfter1 != 1 {
		t.Fatalf("FAIL: tx1 should have been admitted to mempool, got %d items", lenAfter1)
	}

	// tx2 is a valid independent transaction that would fit if only count was checked,
	// but exceeds the remaining byte budget.
	tx2 := createTestSpendTx(t, chain, "budget-2", 2000, 20)
	tx2Bytes := encodeTxMsg(t, tx2)

	srv.HandleTx(tx2Bytes, peer.ID("peer-2"))

	srv.MempoolMux.Lock()
	lenAfter2 := len(srv.Mempool)
	srv.MempoolMux.Unlock()
	if lenAfter2 != 1 {
		t.Fatalf("FAIL: tx2 should have been rejected due to mempool byte budget exhaustion, but mempool size is %d", lenAfter2)
	}
}

// TestPeerCannotExceedMempoolQuota verifies that an individual peer cannot exceed
// its quota of mempool memory or transaction count, preventing single-peer starvation.
func TestPeerCannotExceedMempoolQuota(t *testing.T) {
	chain := newTestBlockchain(t)
	utxo := &storage.UTXOSet{Blockchain: chain}
	srv := makeMempoolServer(t, chain, utxo)

	peerA := peer.ID("peer-attacker-quota")
	peerB := peer.ID("peer-honest-user")

	txA1 := createTestSpendTx(t, chain, "peer-quota-a1", 1000, 10)
	txA2 := createTestSpendTx(t, chain, "peer-quota-a2", 2000, 10)
	txB1 := createTestSpendTx(t, chain, "peer-quota-b1", 3000, 10)

	// Set peer quota to only allow one transaction's worth of bytes from peerA
	srv.MaxPeerMempoolBytes = len(txA1.Serialize()) + 10

	// Peer A submits txA1 -> should succeed
	srv.HandleTx(encodeTxMsg(t, txA1), peerA)

	srv.MempoolMux.Lock()
	lenAfterA1 := len(srv.Mempool)
	srv.MempoolMux.Unlock()
	if lenAfterA1 != 1 {
		t.Fatalf("FAIL: txA1 should be admitted, mempool has %d items", lenAfterA1)
	}

	// Peer A attempts to submit txA2 -> should be rejected because peerA exceeded quota
	srv.HandleTx(encodeTxMsg(t, txA2), peerA)

	srv.MempoolMux.Lock()
	lenAfterA2 := len(srv.Mempool)
	srv.MempoolMux.Unlock()
	if lenAfterA2 != 1 {
		t.Fatalf("FAIL: txA2 should be rejected due to peer quota exhaustion, mempool has %d items", lenAfterA2)
	}

	// Peer B submits txB1 -> should be admitted because Peer B has not exceeded quota
	srv.HandleTx(encodeTxMsg(t, txB1), peerB)

	srv.MempoolMux.Lock()
	lenAfterB1 := len(srv.Mempool)
	srv.MempoolMux.Unlock()
	if lenAfterB1 != 2 {
		t.Fatalf("FAIL: txB1 from Peer B should be admitted, mempool has %d items", lenAfterB1)
	}
}
