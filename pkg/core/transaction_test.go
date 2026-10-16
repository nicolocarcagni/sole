package core

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"testing"
	"time"
)

// Helper to generate a keypair for testing
func generateTestKeyPair() (ecdsa.PrivateKey, []byte) {
	curve := elliptic.P256()
	private, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		panic(err)
	}
	pubKey := append([]byte{0x04}, private.PublicKey.X.Bytes()...)
	pubKey = append(pubKey, private.PublicKey.Y.Bytes()...)
	return *private, pubKey
}

func TestTxIDPostSigningAndSerializationConsistency(t *testing.T) {
	privKey, pubKey := generateTestKeyPair()
	pubKeyHash := HashPubKey(pubKey)

	// Create a dummy previous transaction
	prevTxID, _ := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000001")
	prevTx := Transaction{
		ID: prevTxID,
		Vout: []TxOutput{
			{Value: 100, PubKeyHash: pubKeyHash},
		},
	}

	prevTXs := map[string]Transaction{
		hex.EncodeToString(prevTxID): prevTx,
	}

	inputs := []TxInput{
		{Txid: prevTxID, Vout: 0, Signature: nil, PubKey: pubKey},
	}

	// Destination keypair
	_, destPubKey := generateTestKeyPair()
	destPubKeyHash := HashPubKey(destPubKey)

	outputs := []TxOutput{
		{Value: 50, PubKeyHash: destPubKeyHash},
		{Value: 50, PubKeyHash: pubKeyHash},
	}

	tx := Transaction{
		ID:        nil,
		Vin:       inputs,
		Vout:      outputs,
		Timestamp: time.Now().Unix(),
	}

	// 1. Sign the transaction
	tx.Sign(privKey, prevTXs)

	// Assert that tx.ID is non-empty and equals tx.Hash()
	if len(tx.ID) == 0 {
		t.Fatalf("Expected tx.ID to be populated after signing, got empty")
	}
	if !bytes.Equal(tx.ID, tx.Hash()) {
		t.Fatalf("Expected tx.ID to equal tx.Hash(), got %x != %x", tx.ID, tx.Hash())
	}

	// 2. Serialize and Deserialize
	serialized := tx.Serialize()
	deserializedTx, err := DeserializeTransaction(serialized)
	if err != nil {
		t.Fatalf("deserialize signed transaction: %v", err)
	}

	// Assert bytes.Equal(tx.ID, deserializedTx.ID) is true
	if !bytes.Equal(tx.ID, deserializedTx.ID) {
		t.Fatalf("Serialization invariant failed: original ID %x != deserialized ID %x", tx.ID, deserializedTx.ID)
	}
}

func TestSerializeForHashDistinguishesDifferentStructures(t *testing.T) {
	txA := Transaction{
		Vin: []TxInput{{
			Txid:      []byte{0x01},
			Vout:      2,
			Signature: []byte{0x04},
			PubKey:    []byte{0x03},
		}},
		Timestamp: 5,
	}
	txB := Transaction{
		Vin: []TxInput{{
			Txid:   []byte{0x01},
			Vout:   2,
			PubKey: []byte{0x03, 0x04},
		}},
		Timestamp: 5,
	}

	if bytes.Equal(txA.SerializeForHash(), txB.SerializeForHash()) {
		t.Fatal("different input field boundaries produced the same hash preimage")
	}
	if bytes.Equal(txA.Hash(), txB.Hash()) {
		t.Fatal("different transaction structures produced the same transaction ID")
	}
}

func TestTransactionIDCanonicalEncoding(t *testing.T) {
	tx := Transaction{
		Vin: []TxInput{{
			Txid:      []byte{0xaa, 0xbb},
			Vout:      1,
			Signature: []byte{0xcc},
			PubKey:    []byte{0xdd, 0xee},
		}},
		Vout:      []TxOutput{{Value: 7, PubKeyHash: []byte{0xff}}},
		Timestamp: 9,
	}

	wantEncoding := []byte{
		0, 0, 0, 0, 0, 0, 0, 1, // input count
		0, 0, 0, 0, 0, 0, 0, 2, 0xaa, 0xbb, // Txid length and bytes
		0, 0, 0, 0, 0, 0, 0, 1, // Vout
		0, 0, 0, 0, 0, 0, 0, 1, 0xcc, // Signature length and bytes
		0, 0, 0, 0, 0, 0, 0, 2, 0xdd, 0xee, // PubKey length and bytes
		0, 0, 0, 0, 0, 0, 0, 1, // output count
		0, 0, 0, 0, 0, 0, 0, 7, // Value
		0, 0, 0, 0, 0, 0, 0, 1, 0xff, // PubKeyHash length and bytes
		0, 0, 0, 0, 0, 0, 0, 9, // Timestamp
	}
	if got := tx.SerializeForHash(); !bytes.Equal(got, wantEncoding) {
		t.Fatalf("unexpected canonical transaction encoding:\n got %x\nwant %x", got, wantEncoding)
	}

	wantID := sha256.Sum256(wantEncoding)
	if got := tx.Hash(); !bytes.Equal(got, wantID[:]) {
		t.Fatalf("transaction ID does not hash the canonical encoding: got %x, want %x", got, wantID)
	}
}

func TestDeserializeTransactionRejectsTruncatedFields(t *testing.T) {
	tx := Transaction{
		Vin:       []TxInput{{Txid: []byte{0x01}, Vout: 2}},
		Vout:      []TxOutput{{Value: 7, PubKeyHash: []byte{0xff}}},
		Timestamp: 9,
	}
	serialized := tx.Serialize()
	for end := 0; end < len(serialized); end++ {
		if _, err := DeserializeTransaction(serialized[:end]); err == nil {
			t.Fatalf("truncated transaction with %d of %d bytes was accepted", end, len(serialized))
		}
	}
}

func TestDeserializeTransactionRejectsTrailingBytes(t *testing.T) {
	tx := Transaction{
		Vin:       []TxInput{{Txid: []byte{0x01}, Vout: 2}},
		Vout:      []TxOutput{{Value: 7, PubKeyHash: []byte{0xff}}},
		Timestamp: 9,
	}
	serialized := append(tx.Serialize(), 0x00)

	_, err := DeserializeTransaction(serialized)
	if err == nil {
		t.Fatal("transaction with trailing bytes was accepted")
	}
}

func TestDeserializeTransactionRoundTrip(t *testing.T) {
	tx := Transaction{
		Vin:       []TxInput{{Txid: []byte{0x01}, Vout: 2, Signature: []byte{0x03}, PubKey: []byte{0x04}}},
		Vout:      []TxOutput{{Value: 7, PubKeyHash: []byte{0xff}}},
		Timestamp: 9,
	}
	serialized := tx.Serialize()

	decoded, err := DeserializeTransaction(serialized)
	if err != nil {
		t.Fatalf("deserialize transaction: %v", err)
	}
	if got := decoded.Serialize(); !bytes.Equal(got, serialized) {
		t.Fatalf("transaction round trip changed serialized bytes: got %x, want %x", got, serialized)
	}
	if !bytes.Equal(decoded.ID, decoded.Hash()) {
		t.Fatalf("decoded transaction ID does not match its canonical hash: got %x, want %x", decoded.ID, decoded.Hash())
	}
}

func TestLowSCanonicalEnforcementAndMalleabilityAttack(t *testing.T) {
	privKey, pubKey := generateTestKeyPair()
	pubKeyHash := HashPubKey(pubKey)

	prevTxID, _ := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000002")
	prevTx := Transaction{
		ID: prevTxID,
		Vout: []TxOutput{
			{Value: 100, PubKeyHash: pubKeyHash},
		},
	}

	prevTXs := map[string]Transaction{
		hex.EncodeToString(prevTxID): prevTx,
	}

	inputs := []TxInput{
		{Txid: prevTxID, Vout: 0, Signature: nil, PubKey: pubKey},
	}

	_, destPubKey := generateTestKeyPair()
	destPubKeyHash := HashPubKey(destPubKey)

	outputs := []TxOutput{
		{Value: 100, PubKeyHash: destPubKeyHash},
	}

	tx := Transaction{
		ID:        nil,
		Vin:       inputs,
		Vout:      outputs,
		Timestamp: time.Now().Unix(),
	}

	tx.Sign(privKey, prevTXs)

	// 1. Verify it passes with original canonical signature
	if !tx.Verify(prevTXs) {
		t.Fatalf("Expected validly signed transaction to verify successfully")
	}

	// 2. Manually malleate the input signature by computing s_malleated = N - s
	sig := tx.Vin[0].Signature
	if len(sig) != 64 {
		t.Fatalf("Expected signature length to be 64, got %d", len(sig))
	}

	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])

	curveOrder := elliptic.P256().Params().N
	sMalleated := new(big.Int).Sub(curveOrder, s)

	rBytes := make([]byte, 32)
	sMalleatedBytes := make([]byte, 32)
	r.FillBytes(rBytes)
	sMalleated.FillBytes(sMalleatedBytes)

	malleatedSig := append(rBytes, sMalleatedBytes...)

	// Replace the signature
	tx.Vin[0].Signature = malleatedSig

	// Recalculate tx.ID to bypass the ID check, so we can test the S malleability check.
	tx.ID = tx.Hash()

	// 3. Assert that tx.Verify() rejects it
	if tx.Verify(prevTXs) {
		t.Fatalf("Expected malleated transaction to fail verification, but it succeeded")
	}
}

func TestMerkleRootPreservation(t *testing.T) {
	// Create two transactions
	privKey, pubKey := generateTestKeyPair()
	pubKeyHash := HashPubKey(pubKey)

	prevTxID1, _ := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000003")
	prevTxID2, _ := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000004")

	prevTx1 := Transaction{ID: prevTxID1, Vout: []TxOutput{{Value: 100, PubKeyHash: pubKeyHash}}}
	prevTx2 := Transaction{ID: prevTxID2, Vout: []TxOutput{{Value: 100, PubKeyHash: pubKeyHash}}}

	prevTXs := map[string]Transaction{
		hex.EncodeToString(prevTxID1): prevTx1,
		hex.EncodeToString(prevTxID2): prevTx2,
	}

	_, destPubKey := generateTestKeyPair()
	destPubKeyHash := HashPubKey(destPubKey)

	tx1 := Transaction{
		Vin:       []TxInput{{Txid: prevTxID1, Vout: 0, PubKey: pubKey}},
		Vout:      []TxOutput{{Value: 100, PubKeyHash: destPubKeyHash}},
		Timestamp: time.Now().Unix(),
	}
	tx1.Sign(privKey, prevTXs)

	tx2 := Transaction{
		Vin:       []TxInput{{Txid: prevTxID2, Vout: 0, PubKey: pubKey}},
		Vout:      []TxOutput{{Value: 100, PubKeyHash: destPubKeyHash}},
		Timestamp: time.Now().Unix(),
	}
	tx2.Sign(privKey, prevTXs)

	transactions := []*Transaction{&tx1, &tx2}

	// 1. Record the Merkle Root of their tx.IDs
	var txIDs [][]byte
	for _, tx := range transactions {
		txIDs = append(txIDs, tx.ID)
	}
	mTree1 := NewMerkleTree(txIDs)
	root1 := mTree1.RootNode.Data

	// 2. Serialize and deserialize all transactions
	var deserializedTXs []*Transaction
	for _, tx := range transactions {
		data := tx.Serialize()
		deserializedTx, err := DeserializeTransaction(data)
		if err != nil {
			t.Fatalf("deserialize transaction: %v", err)
		}
		deserializedTXs = append(deserializedTXs, &deserializedTx)
	}

	// 3. Recompute the Merkle Root from the deserialized transactions' tx.IDs
	var desTxIDs [][]byte
	for _, tx := range deserializedTXs {
		desTxIDs = append(desTxIDs, tx.ID)
	}
	mTree2 := NewMerkleTree(desTxIDs)
	root2 := mTree2.RootNode.Data

	// 4. Assert both roots are identical
	if !bytes.Equal(root1, root2) {
		t.Fatalf("Merkle roots do not match: original %x, deserialized %x", root1, root2)
	}
}
