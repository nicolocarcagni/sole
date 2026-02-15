package main

import (
	"log"
)

const (
	GenesisTimestamp    = 1768947120
	GenesisCoinbaseData = "Lu sule, lu mare, lu ientu. Unisalento 2026."
	GenesisAdminAddress = "1HSYNy8yXUuUZrkBCnzSc34Lqr8soPAKQL"
	GenesisReward       = 5000000
)

// NewGenesisBlock creates and returns the genesis block
func NewGenesisBlock() *Block {
	// Deserialize address
	pubKeyHash, err := Base58Decode([]byte(GenesisAdminAddress))
	if err != nil {
		log.Panic("Invalid Genesis Admin Address:", err)
	}
	// Remove version and checksum
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4]

	// Create Coinbase Transaction manually
	txin := TxInput{[]byte{}, -1, nil, []byte(GenesisCoinbaseData)}
	txout := NewTxOutput(int64(GenesisReward*100000000), GenesisAdminAddress) // 5M * 10^8
	coinbase := &Transaction{[]byte("SOLE_GENESIS_TX_ID"), []TxInput{txin}, []TxOutput{*txout}, int64(GenesisTimestamp)}

	// Hash is usually set by Hash(), but we want fixed ID
	// Check if Hash() logic in transaction.go is compatible or if we force it.
	// The prompt says: "La Transazione Coinbase deve avere un ID fisso... []byte("SOLE_GENESIS_TX_ID")"
	// So we just set it.

	// Create Block
	block := &Block{
		Timestamp:     int64(GenesisTimestamp),
		Transactions:  []*Transaction{coinbase},
		PrevBlockHash: []byte{},
		Hash:          []byte{},
		Height:        0,
		Validator:     []byte("Genesis"),
		Signature:     []byte{}, // No signature for genesis or empty
	}
	MineBlock(block)
	return block
}
