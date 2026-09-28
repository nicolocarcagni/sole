package core

import (
	"bytes"
	"testing"
	"time"
)

// makeTestBlock returns a minimal valid block for testing purposes.
// It does NOT call consensus.MineBlock or sign, so Nonce may not satisfy PoW.
func makeTestBlock(validator []byte) *Block {
	tx := &Transaction{
		ID:  []byte("test-tx-id-0001"),
		Vin: []TxInput{},
		Vout: []TxOutput{
			{Value: 1000, PubKeyHash: []byte("somepubkeyhash")},
		},
		Timestamp: time.Now().Unix(),
	}

	block := &Block{
		Timestamp:     time.Now().Unix(),
		Transactions:  []*Transaction{tx},
		PrevBlockHash: []byte("prevhash0000000000000000000000000"),
		Hash:          []byte{},
		Height:        1,
		Nonce:         42,
		Validator:     validator,
	}
	return block
}

// ---------------------------------------------------------------------------
// CalculateHash determinism
// ---------------------------------------------------------------------------

// TestCalculateHash_Determinism verifies that two calls to CalculateHash on
// the same block (with identical fields) always return the same digest.
func TestCalculateHash_Determinism(t *testing.T) {
	t.Parallel()

	b := makeTestBlock([]byte("validatorkey1234567890123456789012345678901234567890123456789012"))

	h1 := b.CalculateHash()
	h2 := b.CalculateHash()

	if !bytes.Equal(h1, h2) {
		t.Fatalf("CalculateHash is not deterministic: got %x and %x", h1, h2)
	}
	if len(h1) != 32 {
		t.Fatalf("Expected 32-byte SHA-256 digest, got %d bytes", len(h1))
	}
}

// TestCalculateHash_DoesNotMutate verifies that CalculateHash() does not
// modify block.Hash.
func TestCalculateHash_DoesNotMutate(t *testing.T) {
	t.Parallel()

	b := makeTestBlock([]byte("validator"))
	b.Hash = []byte("original-hash")

	_ = b.CalculateHash()

	if !bytes.Equal(b.Hash, []byte("original-hash")) {
		t.Fatal("CalculateHash() must not mutate b.Hash")
	}
}

// TestSetHash_Consistency verifies that after SetHash(), b.Hash equals
// a fresh call to CalculateHash().
func TestSetHash_Consistency(t *testing.T) {
	t.Parallel()

	b := makeTestBlock([]byte("myvalidator"))
	b.SetHash()

	if !bytes.Equal(b.Hash, b.CalculateHash()) {
		t.Fatal("After SetHash(), b.Hash != b.CalculateHash()")
	}
}

// ---------------------------------------------------------------------------
// Avalanche effect: altering any single field must change the hash
// ---------------------------------------------------------------------------

// TestCalculateHash_FieldSensitivity_Nonce verifies that changing the Nonce
// produces a different hash.
func TestCalculateHash_FieldSensitivity_Nonce(t *testing.T) {
	t.Parallel()

	b := makeTestBlock([]byte("validator"))
	b.Nonce = 1
	h1 := b.CalculateHash()

	b.Nonce = 2
	h2 := b.CalculateHash()

	if bytes.Equal(h1, h2) {
		t.Fatal("Expected different hashes for different Nonce values, but they are equal")
	}
}

// TestCalculateHash_FieldSensitivity_Timestamp verifies that changing the
// Timestamp produces a different hash.
func TestCalculateHash_FieldSensitivity_Timestamp(t *testing.T) {
	t.Parallel()

	b := makeTestBlock([]byte("validator"))
	b.Timestamp = 1000000
	h1 := b.CalculateHash()

	b.Timestamp = 1000001
	h2 := b.CalculateHash()

	if bytes.Equal(h1, h2) {
		t.Fatal("Expected different hashes for different Timestamp values, but they are equal")
	}
}

// TestCalculateHash_FieldSensitivity_Height verifies that changing the Height
// produces a different hash.
func TestCalculateHash_FieldSensitivity_Height(t *testing.T) {
	t.Parallel()

	b := makeTestBlock([]byte("validator"))
	b.Height = 5
	h1 := b.CalculateHash()

	b.Height = 6
	h2 := b.CalculateHash()

	if bytes.Equal(h1, h2) {
		t.Fatal("Expected different hashes for different Height values, but they are equal")
	}
}

// TestCalculateHash_FieldSensitivity_PrevBlockHash verifies that changing
// PrevBlockHash produces a different hash.
func TestCalculateHash_FieldSensitivity_PrevBlockHash(t *testing.T) {
	t.Parallel()

	b := makeTestBlock([]byte("validator"))
	b.PrevBlockHash = []byte("hash-A")
	h1 := b.CalculateHash()

	b.PrevBlockHash = []byte("hash-B")
	h2 := b.CalculateHash()

	if bytes.Equal(h1, h2) {
		t.Fatal("Expected different hashes for different PrevBlockHash values, but they are equal")
	}
}

// TestCalculateHash_FieldSensitivity_Validator verifies that changing the
// Validator field produces a different hash. This is the core invariant
// that was broken before the fix.
func TestCalculateHash_FieldSensitivity_Validator(t *testing.T) {
	t.Parallel()

	b := makeTestBlock(nil)
	b.Validator = bytes.Repeat([]byte{0xAA}, 64)
	h1 := b.CalculateHash()

	b.Validator = bytes.Repeat([]byte{0xBB}, 64)
	h2 := b.CalculateHash()

	if bytes.Equal(h1, h2) {
		t.Fatal("Expected different hashes for different Validator values, but they are equal")
	}
}

// TestCalculateHash_FieldSensitivity_TxID verifies that altering a
// transaction ID produces a different hash (via the Merkle root).
func TestCalculateHash_FieldSensitivity_TxID(t *testing.T) {
	t.Parallel()

	b := makeTestBlock([]byte("validator"))
	b.Transactions[0].ID = []byte("tx-id-original")
	h1 := b.CalculateHash()

	b.Transactions[0].ID = []byte("tx-id-tampered")
	h2 := b.CalculateHash()

	if bytes.Equal(h1, h2) {
		t.Fatal("Expected different hashes for different transaction IDs, but they are equal")
	}
}

// TestCalculateHash_NilValidator_vs_EmptyValidator ensures that a nil
// Validator and an empty []byte{} Validator produce the same hash (both
// serialize as zero-length byte slices in bytes.Join).
func TestCalculateHash_NilValidator_vs_EmptyValidator(t *testing.T) {
	t.Parallel()

	b1 := makeTestBlock(nil)
	b2 := makeTestBlock([]byte{})

	// Ensure all other fields are identical
	b2.Timestamp = b1.Timestamp
	b2.Nonce = b1.Nonce

	h1 := b1.CalculateHash()
	h2 := b2.CalculateHash()

	if !bytes.Equal(h1, h2) {
		t.Fatalf("nil Validator and empty []byte Validator should produce the same hash; got %x and %x", h1, h2)
	}
}

// TestCalculateHash_SignatureExcluded verifies that the Signature field does
// NOT affect the hash (malleability protection).
func TestCalculateHash_SignatureExcluded(t *testing.T) {
	t.Parallel()

	b := makeTestBlock([]byte("validator"))
	b.Signature = []byte{} // no signature
	h1 := b.CalculateHash()

	b.Signature = bytes.Repeat([]byte{0xFF}, 64) // populated signature
	h2 := b.CalculateHash()

	if !bytes.Equal(h1, h2) {
		t.Fatal("Signature must NOT influence CalculateHash(); the hashes differ")
	}
}
