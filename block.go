package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"log"
	"time"
)

// Block represents a block in the blockchain
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

// Serialize serializes the block into a byte slice
func (b *Block) Serialize() []byte {
	var result bytes.Buffer
	encoder := gob.NewEncoder(&result)

	err := encoder.Encode(b)
	if err != nil {
		log.Panic(err)
	}

	return result.Bytes()
}

// SetHash calculates and sets the deterministic SHA-256 hash of the block header.
// It explicitly excludes the Signature field to prevent malleability.
func (b *Block) SetHash() {
	var txHashes [][]byte
	for _, tx := range b.Transactions {
		txHashes = append(txHashes, tx.ID)
	}

	var merkleRoot []byte
	if len(txHashes) > 0 {
		mTree := NewMerkleTree(txHashes)
		merkleRoot = mTree.RootNode.Data
	} else {
		merkleRoot = []byte{}
	}

	timestampBytes := IntToHex(b.Timestamp)
	heightBytes := IntToHex(int64(b.Height))
	nonceBytes := IntToHex(int64(b.Nonce))

	headers := bytes.Join(
		[][]byte{
			b.PrevBlockHash,
			merkleRoot,
			timestampBytes,
			heightBytes,
			nonceBytes,
			b.Validator,
		},
		[]byte{},
	)

	hash := sha256.Sum256(headers)
	b.Hash = hash[:]
}

// HashTransactions returns the Merkle Root of the block's transactions.
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

// NewBlock creates and returns a new Block
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
