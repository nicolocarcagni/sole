package consensus_test

import (
	"github.com/nicolocarcagni/sole/pkg/consensus"

	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v3"

	"github.com/nicolocarcagni/sole/pkg/core"
	"github.com/nicolocarcagni/sole/pkg/storage"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// generateTestKey creates a fresh P-256 ECDSA key pair for testing.
func generateTestKey(t *testing.T) ecdsa.PrivateKey {
	t.Helper()
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ECDSA key: %v", err)
	}
	return *privKey
}

// authorizeKey temporarily adds the 65-byte (0x04‖X‖Y) hex of a key to
// consensus.AuthorizedValidators and schedules its removal via t.Cleanup.
// Returns the 64-byte raw key (X‖Y) suitable for block.Validator.
func authorizeKey(t *testing.T, privKey ecdsa.PrivateKey) []byte {
	t.Helper()
	rawKey := append(
		privKey.PublicKey.X.FillBytes(make([]byte, 32)),
		privKey.PublicKey.Y.FillBytes(make([]byte, 32))...,
	)
	fullKey := append([]byte{0x04}, rawKey...)
	hexKey := hex.EncodeToString(fullKey)

	consensus.AuthorizedValidators = append(consensus.AuthorizedValidators, hexKey)
	t.Cleanup(func() {
		for i, v := range consensus.AuthorizedValidators {
			if v == hexKey {
				consensus.AuthorizedValidators = append(consensus.AuthorizedValidators[:i], consensus.AuthorizedValidators[i+1:]...)
				break
			}
		}
	})
	return rawKey
}

// forgeBlock runs the complete forge-mine-sign pipeline without a DB:
// it constructs a block with valPubKey already set, mines it, and signs it.
func forgeBlock(t *testing.T, privKey ecdsa.PrivateKey, valPubKey []byte, prevHash []byte, height int) *core.Block {
	t.Helper()
	tx := &core.Transaction{
		ID:  []byte("test-coinbase-tx-" + string(rune(height))),
		Vin: []core.TxInput{{Txid: []byte{}, Vout: -1, Signature: nil, PubKey: []byte("cb")}},
		Vout: []core.TxOutput{
			{Value: 1_000_000_000, PubKeyHash: []byte("recipient-ph")},
		},
		Timestamp: time.Now().Unix(),
	}

	block := core.NewBlock([]*core.Transaction{tx}, prevHash, height, valPubKey)
	consensus.MineBlock(block)
	if err := consensus.SignBlock(block, privKey); err != nil {
		t.Fatalf("consensus.SignBlock failed: %v", err)
	}
	return block
}

// makePrevBlock returns a placeholder "previous" block for consensus.ValidateBlockHeader.
func makePrevBlock() *core.Block {
	return &core.Block{
		Timestamp:     time.Now().Unix() - 10,
		PrevBlockHash: []byte{},
		Hash:          bytes.Repeat([]byte{0x01}, 32),
		Height:        0,
	}
}

// ---------------------------------------------------------------------------
// Happy-path: full forge-mine-sign pipeline
// ---------------------------------------------------------------------------

// TestForgeBlock_HashConsistency is the primary regression test for the
// P0 vulnerability. It verifies that after the forge-mine-sign pipeline:
//
//	block.Hash == block.CalculateHash()
//
// This invariant was broken before the fix because the hash was computed
// without the Validator field.
func TestForgeBlock_HashConsistency(t *testing.T) {
	privKey := generateTestKey(t)
	valPubKey := authorizeKey(t, privKey)
	prevHash := bytes.Repeat([]byte{0x01}, 32)

	block := forgeBlock(t, privKey, valPubKey, prevHash, 1)

	if !bytes.Equal(block.Hash, block.CalculateHash()) {
		t.Fatalf("REGRESSION: block.Hash != block.CalculateHash() after forge-mine-sign pipeline\nstored:     %x\ncalculated: %x",
			block.Hash, block.CalculateHash())
	}
}

// TestForgeBlock_ValidatorSet verifies that block.Validator is set to the
// correct 64-byte raw public key after forging.
func TestForgeBlock_ValidatorSet(t *testing.T) {
	privKey := generateTestKey(t)
	valPubKey := authorizeKey(t, privKey)
	prevHash := bytes.Repeat([]byte{0x01}, 32)

	block := forgeBlock(t, privKey, valPubKey, prevHash, 1)

	if len(block.Validator) != 64 {
		t.Fatalf("Expected 64-byte Validator, got %d bytes", len(block.Validator))
	}
	if !bytes.Equal(block.Validator, valPubKey) {
		t.Fatal("block.Validator does not match the expected public key")
	}
}

// TestForgeBlock_ProofOfWork verifies that the forged block satisfies the
// anti-spam PoW constraint.
func TestForgeBlock_ProofOfWork(t *testing.T) {
	privKey := generateTestKey(t)
	valPubKey := authorizeKey(t, privKey)
	prevHash := bytes.Repeat([]byte{0x01}, 32)

	block := forgeBlock(t, privKey, valPubKey, prevHash, 1)

	if !consensus.CheckProofOfWork(block.Hash) {
		t.Fatalf("Forged block fails PoW check. Hash: %x", block.Hash)
	}
}

// TestForgeBlock_SignatureValid verifies that consensus.VerifyBlockSignature succeeds
// on a correctly forged block.
func TestForgeBlock_SignatureValid(t *testing.T) {
	privKey := generateTestKey(t)
	valPubKey := authorizeKey(t, privKey)
	prevHash := bytes.Repeat([]byte{0x01}, 32)

	block := forgeBlock(t, privKey, valPubKey, prevHash, 1)

	if !consensus.VerifyBlockSignature(block) {
		t.Fatal("consensus.VerifyBlockSignature returned false on a correctly forged block")
	}
}

// ---------------------------------------------------------------------------
// consensus.SignBlock — validator mismatch
// ---------------------------------------------------------------------------

// TestSignBlock_ValidatorMismatch verifies that consensus.SignBlock returns an error
// when block.Validator is already bound to a different key than privKey.
func TestSignBlock_ValidatorMismatch(t *testing.T) {
	key1 := generateTestKey(t)
	key2 := generateTestKey(t)

	// Bind key1's pubkey to the block.
	valKey1 := append(
		key1.PublicKey.X.FillBytes(make([]byte, 32)),
		key1.PublicKey.Y.FillBytes(make([]byte, 32))...,
	)
	b := makeTestBlock(valKey1)
	b.SetHash()

	// Attempt to sign with key2 — should fail with a key-mismatch error.
	err := consensus.SignBlock(b, key2)
	if err == nil {
		t.Fatal("Expected consensus.SignBlock to return an error for key mismatch, but it succeeded")
	}
}

// TestSignBlock_SetsHashWhenValidatorEmpty verifies that consensus.SignBlock correctly
// sets the Validator and recomputes the hash when block.Validator is empty.
func TestSignBlock_SetsHashWhenValidatorEmpty(t *testing.T) {
	key := generateTestKey(t)
	b := makeTestBlock(nil) // no validator
	b.Hash = []byte{}

	if err := consensus.SignBlock(b, key); err != nil {
		t.Fatalf("consensus.SignBlock (no prior Validator) failed: %v", err)
	}

	if len(b.Validator) != 64 {
		t.Fatalf("Expected Validator to be set to 64 bytes, got %d", len(b.Validator))
	}
	if !bytes.Equal(b.Hash, b.CalculateHash()) {
		t.Fatal("After consensus.SignBlock (no prior Validator), block.Hash != block.CalculateHash()")
	}
}

// ---------------------------------------------------------------------------
// consensus.ValidateBlockHeader — strict hash enforcement
// ---------------------------------------------------------------------------

// TestValidateBlockHeader_ValidBlock verifies that a correctly forged block
// passes header validation.
func TestValidateBlockHeader_ValidBlock(t *testing.T) {
	privKey := generateTestKey(t)
	valPubKey := authorizeKey(t, privKey)
	prevHash := bytes.Repeat([]byte{0x01}, 32)

	block := forgeBlock(t, privKey, valPubKey, prevHash, 1)
	prev := makePrevBlock()

	if err := consensus.ValidateBlockHeader(block, prev); err != nil {
		t.Fatalf("Expected valid forged block to pass consensus.ValidateBlockHeader, but got: %v", err)
	}
}

// TestValidateBlockHeader_TamperedTransaction verifies that altering a
// transaction ID after mining causes consensus.ValidateBlockHeader to reject the block.
func TestValidateBlockHeader_TamperedTransaction(t *testing.T) {
	privKey := generateTestKey(t)
	valPubKey := authorizeKey(t, privKey)
	prevHash := bytes.Repeat([]byte{0x01}, 32)

	block := forgeBlock(t, privKey, valPubKey, prevHash, 1)
	prev := makePrevBlock()

	// Tamper: modify the transaction ID — the Merkle root (and thus hash) changes.
	block.Transactions[0].ID = []byte("tampered-tx-id!!")

	if err := consensus.ValidateBlockHeader(block, prev); err == nil {
		t.Fatal("Expected consensus.ValidateBlockHeader to reject block with tampered transaction, but it passed")
	}
}

// TestValidateBlockHeader_TamperedValidator verifies that swapping the
// Validator after signing causes header validation to fail.
func TestValidateBlockHeader_TamperedValidator(t *testing.T) {
	privKey := generateTestKey(t)
	valPubKey := authorizeKey(t, privKey)
	prevHash := bytes.Repeat([]byte{0x01}, 32)

	block := forgeBlock(t, privKey, valPubKey, prevHash, 1)
	prev := makePrevBlock()

	// Replace Validator with an unrelated key (block.Hash is now stale).
	otherKey := generateTestKey(t)
	block.Validator = append(
		otherKey.PublicKey.X.FillBytes(make([]byte, 32)),
		otherKey.PublicKey.Y.FillBytes(make([]byte, 32))...,
	)

	if err := consensus.ValidateBlockHeader(block, prev); err == nil {
		t.Fatal("Expected consensus.ValidateBlockHeader to reject block with swapped Validator, but it passed")
	}
}

// TestValidateBlockHeader_FakePoWHash verifies that a fake hash starting with
// 0x00 (satisfying naive PoW) is rejected because it doesn't match CalculateHash().
func TestValidateBlockHeader_FakePoWHash(t *testing.T) {
	privKey := generateTestKey(t)
	valPubKey := authorizeKey(t, privKey)
	prevHash := bytes.Repeat([]byte{0x01}, 32)

	block := forgeBlock(t, privKey, valPubKey, prevHash, 1)
	prev := makePrevBlock()

	// Craft a fake hash that starts with 0x00 but is otherwise arbitrary.
	fakeHash := make([]byte, 32)
	fakeHash[0] = 0x00
	fakeHash[1] = 0xCA
	fakeHash[2] = 0xFE
	block.Hash = fakeHash

	if err := consensus.ValidateBlockHeader(block, prev); err == nil {
		t.Fatal("Expected consensus.ValidateBlockHeader to reject a fake PoW hash, but it passed")
	}
}

// TestValidateBlockHeader_ValidPoWHashButWrongHeader verifies that even a
// hash that satisfies PoW but was computed from a different header is rejected.
func TestValidateBlockHeader_ValidPoWHashButWrongHeader(t *testing.T) {
	privKey := generateTestKey(t)
	valPubKey := authorizeKey(t, privKey)
	prevHash := bytes.Repeat([]byte{0x01}, 32)

	block := forgeBlock(t, privKey, valPubKey, prevHash, 1)
	prev := makePrevBlock()

	// Build a different block and mine it to obtain a foreign PoW-valid hash.
	foreignBlock := makeTestBlock([]byte("foreign-validator-bytes"))
	foreignBlock.Nonce = 0
	for !consensus.CheckProofOfWork(foreignBlock.CalculateHash()) {
		foreignBlock.Nonce++
	}
	foreignBlock.SetHash()

	// Assign the foreign hash to our real block — it satisfies PoW but doesn't
	// match this block's CalculateHash().
	block.Hash = foreignBlock.Hash

	if err := consensus.ValidateBlockHeader(block, prev); err == nil {
		t.Fatal("Expected consensus.ValidateBlockHeader to reject a PoW-valid but header-mismatching hash, but it passed")
	}
}

// ---------------------------------------------------------------------------
// consensus.VerifyBlockSignature — invalid / corrupted signature
// ---------------------------------------------------------------------------

// TestVerifyBlockSignature_CorruptedSignature verifies that flipping bytes in
// the ECDSA signature causes consensus.VerifyBlockSignature to return false.
func TestVerifyBlockSignature_CorruptedSignature(t *testing.T) {
	privKey := generateTestKey(t)
	valPubKey := authorizeKey(t, privKey)
	prevHash := bytes.Repeat([]byte{0x01}, 32)

	block := forgeBlock(t, privKey, valPubKey, prevHash, 1)

	// Flip bytes in the signature.
	block.Signature[0] ^= 0xFF
	block.Signature[31] ^= 0xFF

	if consensus.VerifyBlockSignature(block) {
		t.Fatal("Expected consensus.VerifyBlockSignature to return false for a corrupted signature, but it returned true")
	}
}

// TestVerifyBlockSignature_HashTamperedAfterSigning verifies that changing
// block.Hash after signing (without re-signing) fails signature verification.
func TestVerifyBlockSignature_HashTamperedAfterSigning(t *testing.T) {
	privKey := generateTestKey(t)
	valPubKey := authorizeKey(t, privKey)
	prevHash := bytes.Repeat([]byte{0x01}, 32)

	block := forgeBlock(t, privKey, valPubKey, prevHash, 1)

	// Flip the last byte of the stored hash.
	block.Hash[len(block.Hash)-1] ^= 0xFF

	if consensus.VerifyBlockSignature(block) {
		t.Fatal("Expected consensus.VerifyBlockSignature to fail when hash is tampered after signing, but it returned true")
	}
}

// ---------------------------------------------------------------------------
// AddBlock — hash integrity gate (integration, requires BadgerDB)
// ---------------------------------------------------------------------------

// openTestDB opens a BadgerDB in the given directory for integration tests.
func openTestDB(t *testing.T, dir string) *badger.DB {
	t.Helper()
	opts := storage.GetBadgerOptions(dir)
	db, err := badger.Open(opts)
	if err != nil {
		t.Skipf("BadgerDB unavailable in test environment: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// initTestChain stores the genesis block in db and returns a storage.Blockchain.
func initTestChain(t *testing.T, db *badger.DB) *storage.Blockchain {
	t.Helper()
	genesis, _ := storage.NewGenesisBlock()
	err := db.Update(func(txn *badger.Txn) error {
		serialized, err := genesis.Serialize()
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		if err := txn.Set(genesis.Hash, serialized); err != nil {
			return err
		}
		for _, tx := range genesis.Transactions {
			if err := txn.Set(append([]byte("tx-"), tx.ID...), genesis.Hash); err != nil {
				return err
			}
		}
		return txn.Set([]byte("lh"), genesis.Hash)
	})
	if err != nil {
		t.Fatalf("failed to initialise test chain: %v", err)
	}
	return &storage.Blockchain{LastHash: genesis.Hash, Database: db}
}

// TestAddBlock_AcceptsValidBlock verifies that a correctly forged block is
// accepted by AddBlock (happy path integration test).
func TestAddBlock_AcceptsValidBlock(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "blocks")
	if err := os.MkdirAll(dataDir, os.ModePerm); err != nil {
		t.Skipf("cannot create temp data dir: %v", err)
	}

	db := openTestDB(t, dataDir)
	chain := initTestChain(t, db)

	privKey := generateTestKey(t)
	valPubKey := authorizeKey(t, privKey)

	genesis, _ := storage.NewGenesisBlock()

	tx := &core.Transaction{
		ID:        []byte("addblock-valid-tx"),
		Vin:       []core.TxInput{{Txid: []byte{}, Vout: -1, Signature: nil, PubKey: []byte("cb")}},
		Vout:      []core.TxOutput{{Value: 500, PubKeyHash: []byte("ph")}},
		Timestamp: time.Now().Unix(),
	}
	block := core.NewBlock([]*core.Transaction{tx}, chain.LastHash, 1, valPubKey)
	// Timestamp must be strictly greater than genesis.
	block.Timestamp = genesis.Timestamp + 1
	block.SetHash()
	consensus.MineBlock(block)
	if err := consensus.SignBlock(block, privKey); err != nil {
		t.Fatalf("consensus.SignBlock: %v", err)
	}

	if !chain.AddBlock(block) {
		t.Fatal("AddBlock rejected a valid block")
	}
}

// TestAddBlock_RejectsHashMismatch verifies that AddBlock rejects a block
// whose Validator was swapped after mining (hash doesn't match CalculateHash).
func TestAddBlock_RejectsHashMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "blocks")
	if err := os.MkdirAll(dataDir, os.ModePerm); err != nil {
		t.Skipf("cannot create temp data dir: %v", err)
	}

	db := openTestDB(t, dataDir)
	chain := initTestChain(t, db)

	privKey := generateTestKey(t)
	valPubKey := authorizeKey(t, privKey)

	genesis, _ := storage.NewGenesisBlock()

	tx := &core.Transaction{
		ID:        []byte("addblock-tampered-tx"),
		Vin:       []core.TxInput{{Txid: []byte{}, Vout: -1, Signature: nil, PubKey: []byte("cb")}},
		Vout:      []core.TxOutput{{Value: 500, PubKeyHash: []byte("ph")}},
		Timestamp: time.Now().Unix(),
	}
	block := core.NewBlock([]*core.Transaction{tx}, chain.LastHash, 1, valPubKey)
	block.Timestamp = genesis.Timestamp + 1
	block.SetHash()
	consensus.MineBlock(block)
	if err := consensus.SignBlock(block, privKey); err != nil {
		t.Fatalf("consensus.SignBlock: %v", err)
	}

	// Tamper: swap Validator to a different key (hash is now stale/mismatching).
	otherKey := generateTestKey(t)
	block.Validator = append(
		otherKey.PublicKey.X.FillBytes(make([]byte, 32)),
		otherKey.PublicKey.Y.FillBytes(make([]byte, 32))...,
	)

	if chain.AddBlock(block) {
		t.Fatal("AddBlock accepted a block with a tampered Validator — hash integrity check failed")
	}
}

func makeTestBlock(validator []byte) *core.Block {
	tx := &core.Transaction{
		ID:  []byte("test-tx-id-0001"),
		Vin: []core.TxInput{},
		Vout: []core.TxOutput{
			{Value: 1000, PubKeyHash: []byte("somepubkeyhash")},
		},
		Timestamp: 123456789,
	}

	block := &core.Block{
		Timestamp:     123456789,
		Transactions:  []*core.Transaction{tx},
		PrevBlockHash: []byte("prevhash0000000000000000000000000"),
		Hash:          []byte{},
		Height:        1,
		Validator:     validator,
		Nonce:         0,
		Signature:     []byte("test-sig"),
	}
	block.Hash = block.CalculateHash()
	return block
}
