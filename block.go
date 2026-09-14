package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"fmt"
	"time"
)

type Block struct {
	Timestamp     int64
	Transactions  []*Transaction
	PrevBlockHash []byte
	Hash          []byte
	Height        int
	Nonce         int    // PoA Anti-Spam
	Validator     []byte // Public key of the block validator (64 bytes)
	Signature     []byte // ECDSA signature of the block hash (64 bytes)
}

func (b *Block) Serialize() ([]byte, error) {
	var result bytes.Buffer
	encoder := gob.NewEncoder(&result)

	err := encoder.Encode(b)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize block: %w", err)
	}

	return result.Bytes(), nil
}

// CalculateHash computes the deterministic SHA-256 digest of the block header
// without mutating any field. The preimage commits to every header field that
// must be tamper-evident: PrevBlockHash, the Merkle root of all transaction
// IDs, Timestamp, Height, Nonce, and the validator's public key.
// The Signature field is intentionally excluded to prevent malleability.
func (b *Block) CalculateHash() []byte {
	merkleRoot := b.HashTransactions()

	headers := bytes.Join(
		[][]byte{
			b.PrevBlockHash,
			merkleRoot,
			IntToHex(b.Timestamp),
			IntToHex(int64(b.Height)),
			IntToHex(int64(b.Nonce)),
			b.Validator,
		},
		[]byte{},
	)

	hash := sha256.Sum256(headers)
	return hash[:]
}

// SetHash calculates and sets the deterministic SHA-256 hash of the block header.
// It explicitly excludes the Signature field to prevent malleability.
func (b *Block) SetHash() {
	b.Hash = b.CalculateHash()
}

func (b *Block) HashTransactions() []byte {
	var txHashes [][]byte
	for _, tx := range b.Transactions {
		txHashes = append(txHashes, tx.ID)
	}
	if len(txHashes) == 0 {
		return []byte{}
	}
	mTree := NewMerkleTree(txHashes)
	return mTree.RootNode.Data
}

func NewBlock(transactions []*Transaction, prevBlockHash []byte, height int, validator []byte) *Block {
	block := &Block{
		Timestamp:     time.Now().Unix(),
		Transactions:  transactions,
		PrevBlockHash: prevBlockHash,
		Hash:          []byte{},
		Height:        height,
		Nonce:         0,
		Validator:     validator,
	}
	block.SetHash()
	return block
}
