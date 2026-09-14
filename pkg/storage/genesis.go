package storage

import (
	"fmt"
	"github.com/nicolocarcagni/sole/pkg/consensus"
	"github.com/nicolocarcagni/sole/pkg/core"
)

func NewGenesisBlock() (*core.Block, error) {
	pubKeyHash, err := core.ExtractPubKeyHash(core.GenesisAdminAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid genesis admin address: %w", err)
	}

	txin := core.TxInput{[]byte{}, -1, nil, []byte(core.GenesisCoinbaseData)}
	txout, err := core.NewTxOutput(int64(core.GenesisReward*100000000), core.GenesisAdminAddress) 
	if err != nil {
		return nil, fmt.Errorf("failed to create genesis output: %w", err)
	}
	txout.PubKeyHash = pubKeyHash
	coinbase := &core.Transaction{[]byte("SOLE_GENESIS_TX_ID"), []core.TxInput{txin}, []core.TxOutput{*txout}, int64(core.GenesisTimestamp)}

	block := &core.Block{
		Timestamp:     int64(core.GenesisTimestamp),
		Transactions:  []*core.Transaction{coinbase},
		PrevBlockHash: []byte{},
		Hash:          []byte{},
		Height:        0,
		Validator:     []byte("Genesis"),
		Signature:     []byte{}, 
	}
	consensus.MineBlock(block)
	return block, nil
}
