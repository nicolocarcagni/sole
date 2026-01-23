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

// SetHash calculates and sets the hash of the block
func (b *Block) SetHash() {
	// timestamp := []byte(string(rune(b.Timestamp))) - Removed unused variable
	// Ideally we serialize the whole block to hash it, excluding the hash itself.
	// For simplicity matching the request:

	// Let's use a proper serialization of headers for hashing usually,
	// but here we can just hash the serialized body or specific fields.
	// The prompt asked for SetHash implementation.

	// A simple approach:
	headers := bytes.Join(
		[][]byte{
			b.PrevBlockHash,
			b.HashTransactions(), // We need a way to hash transactions
			IntToHex(b.Timestamp),
			IntToHex(int64(b.Height)),
			b.Validator,
		},
		[]byte{},
	)
	hash := sha256.Sum256(headers)
	b.Hash = hash[:]
}

// HashTransactions returns a hash of the transactions in the block
func (b *Block) HashTransactions() []byte {
	var txHashes [][]byte
	var txHash [32]byte

	for _, tx := range b.Transactions {
		txHashes = append(txHashes, tx.ID)
	}
	// Simple concatenation for now (Merkle Tree is better but complex for this stage)
	// If no txs (unlikely in real blocks but possible in empty ones), handle gracefully
	if len(txHashes) == 0 {
		return []byte{}
	}

	txHash = sha256.Sum256(bytes.Join(txHashes, []byte{}))
	return txHash[:]
}

// NewBlock creates and returns a new Block
func NewBlock(transactions []*Transaction, prevBlockHash []byte, height int, validator []byte) *Block {
	block := &Block{
		Timestamp:     time.Now().Unix(),
		Transactions:  transactions,
		PrevBlockHash: prevBlockHash,
		Hash:          []byte{},
		Height:        height,
		Validator:     validator,
	}
	block.SetHash()
	return block
}
