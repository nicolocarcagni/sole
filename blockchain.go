package main

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"

	"github.com/dgraph-io/badger/v3"
)

const (
	dbPath = "./data/blocks"
)

func getBadgerOptions(path string) badger.Options {
	opts := badger.DefaultOptions(path)
	opts.Logger = nil
	// opts.Truncate = true (Removed in v3)

	opts.ValueLogFileSize = 16 << 20 // 16 MB max value log file size
	opts.MemTableSize = 8 << 20      // 8 MB memtable
	opts.BlockCacheSize = 1 << 20    // 1 MB cache
	opts.NumVersionsToKeep = 1

	// Robustness
	opts.VerifyValueChecksum = true
	opts.DetectConflicts = true

	// Note: Badger v3 removed explicit FileIO/Mmap flags in Options struct.
	// It manages memory mapping internally. On Windows, ensure OS handles mmap correctly.
	if runtime.GOOS == "windows" {
		fmt.Println("🔧 Windows detected: Running with standard Badger v3 defaults.")
	}

	return opts
}

// Blockchain keeps a sequence of Blocks
type Blockchain struct {
	LastHash []byte
	Database *badger.DB
	Mux      sync.Mutex
}

// BlockchainIterator is used to iterate over blockchain blocks
type BlockchainIterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

// InitBlockchain creates a new blockchain with Genesis Block
func InitBlockchain() (*Blockchain, error) {
	var lastHash []byte

	if DBExists() {
		return nil, fmt.Errorf("blockchain already exists")
	}

	// Ensure data directory exists
	if err := os.MkdirAll(dbPath, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %s", err)
	}

	opts := getBadgerOptions(dbPath)

	db, err := badger.Open(opts)
	if err != nil {
		log.Panic(err)
	}

	err = db.Update(func(txn *badger.Txn) error {
		genesis := NewGenesisBlock()
		fmt.Println("Genesis Block created")

		err = txn.Set(genesis.Hash, genesis.Serialize())
		if err != nil {
			log.Panic(err)
		}
		err = txn.Set([]byte("lh"), genesis.Hash)
		lastHash = genesis.Hash
		return err
	})
	if err != nil {
		log.Panic(err)
	}

	blockchain := Blockchain{lastHash, db, sync.Mutex{}}
	return &blockchain, nil
}

// ContinueBlockchain continues an existing blockchain
func ContinueBlockchain(address string) *Blockchain {
	if !DBExists() {
		fmt.Println("No existing blockchain found. Create one first.")
		os.Exit(1)
	}

	var lastHash []byte
	opts := badger.DefaultOptions(dbPath)
	opts.Logger = nil

	db, err := badger.Open(opts)
	if err != nil {
		log.Panic(err)
	}

	err = db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			log.Panic(err)
		}
		lastHash, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		log.Panic(err)
	}

	chain := Blockchain{lastHash, db, sync.Mutex{}}
	return &chain
}

// ContinueBlockchainReadOnly continues an existing blockchain in Read-Only mode
func ContinueBlockchainReadOnly(address string) *Blockchain {
	if !DBExists() {
		fmt.Println("No existing blockchain found. Create one first.")
		os.Exit(1)
	}

	var lastHash []byte
	opts := badger.DefaultOptions(dbPath)
	opts.Logger = nil
	opts.ReadOnly = true // Enable ReadOnly

	db, err := badger.Open(opts)
	if err != nil {
		log.Panic(err)
	}

	err = db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			log.Panic(err)
		}
		lastHash, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		log.Panic(err)
	}

	chain := Blockchain{lastHash, db, sync.Mutex{}}
	return &chain
}

// ContinueBlockchainSnapshot continues a blockchain from a specific path
func ContinueBlockchainSnapshot(customPath string) *Blockchain {
	if _, err := os.Stat(customPath + "/MANIFEST"); os.IsNotExist(err) {
		log.Panic("Snapshot DB corrupt or missing")
	}

	var lastHash []byte
	opts := badger.DefaultOptions(customPath)
	opts.Logger = nil
	// Memory optimizations
	opts.ValueLogFileSize = 16 << 20
	opts.MemTableSize = 8 << 20
	opts.BlockCacheSize = 1 << 20
	opts.NumVersionsToKeep = 1

	db, err := badger.Open(opts)
	if err != nil {
		log.Panic(err)
	}

	err = db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			log.Panic(err)
		}
		lastHash, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		log.Panic(err)
	}

	chain := Blockchain{lastHash, db, sync.Mutex{}}
	return &chain
}

// GetBlock finds a block by hash and returns it
func (chain *Blockchain) GetBlock(blockHash []byte) (Block, error) {
	var block Block

	err := chain.Database.View(func(txn *badger.Txn) error {
		if item, err := txn.Get(blockHash); err != nil {
			return errors.New("Block is not found")
		} else {
			blockData, _ := item.ValueCopy(nil)
			block = *DeserializeBlock(blockData)
		}
		return nil
	})
	return block, err
}

// GetBlockHashes returns a list of hashes of all the blocks in the chain
func (chain *Blockchain) GetBlockHashes() [][]byte { // TODO: Optimization?
	var blocks [][]byte

	// We iterate backwards, so we'll get hashes from Tip to Genesis
	iter := chain.Iterator()

	for {
		block := iter.Next()
		blocks = append(blocks, block.Hash)

		if len(block.PrevBlockHash) == 0 {
			break
		}
	}

	return blocks
}

// GetBestHeight returns the height of the latest block
func (chain *Blockchain) GetBestHeight() int {
	chain.Mux.Lock()
	defer chain.Mux.Unlock()

	var lastBlock Block
	// Logic: fetch last hash, get block, return height.

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			return err
		}
		lastHash, _ := item.ValueCopy(nil)

		item, err = txn.Get(lastHash)
		if err != nil {
			return err
		}
		data, _ := item.ValueCopy(nil)
		lastBlock = *DeserializeBlock(data)
		return nil
	})
	if err != nil {
		return 0
	}

	return lastBlock.Height
}

// ForgeBlock forges a new block with PoA signing
func (chain *Blockchain) ForgeBlock(transactions []*Transaction, privKey ecdsa.PrivateKey) *Block {
	chain.Mux.Lock()
	defer chain.Mux.Unlock()

	var lastHash []byte

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			log.Panic(err)
		}
		lastHash, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		log.Panic(err)
	}

	var lastBlockData []byte
	err = chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get(lastHash)
		if err != nil {
			return err
		}
		lastBlockData, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		log.Panic(err)
	}

	lastBlock := DeserializeBlock(lastBlockData)
	newHeight := lastBlock.Height + 1

	// Create block without signature first
	newBlock := NewBlock(transactions, lastHash, newHeight, nil)

	// Sign the block with validator's private key
	err = SignBlock(newBlock, privKey)
	if err != nil {
		log.Panic("Failed to sign block:", err)
	}

	err = chain.Database.Update(func(txn *badger.Txn) error {
		err := txn.Set(newBlock.Hash, newBlock.Serialize())
		if err != nil {
			log.Panic(err)
		}
		err = txn.Set([]byte("lh"), newBlock.Hash)
		chain.LastHash = newBlock.Hash
		return err
	})
	if err != nil {
		log.Panic(err)
	}

	return newBlock
}

// AddBlock adds a received block to the blockchain after PoA validation
func (chain *Blockchain) AddBlock(block *Block) bool {
	// 0. Exist Check: Verify duplicates BEFORE expensive crypto validation
	_, err := chain.GetBlock(block.Hash)
	if err == nil {
		// fmt.Printf("📦 [Blockchain] Block %x already exists. Skipping.\n", block.Hash[:4])
		return false // Already processed
	}

	// Verify PoA signature first
	if !VerifyBlockSignature(block) {
		fmt.Println("AddBlock: Block rejected - invalid PoA signature")
		return false
	}

	chain.Mux.Lock()
	defer chain.Mux.Unlock()

	err = chain.Database.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(block.Hash); err == nil {
			return nil
		}

		blockData := block.Serialize()
		err := txn.Set(block.Hash, blockData)
		if err != nil {
			return err
		}

		item, err := txn.Get([]byte("lh"))
		if err != nil {
			return err
		}
		lastHash, _ := item.ValueCopy(nil)

		item, err = txn.Get(lastHash)
		if err != nil {
			return err
		}
		lastBlockData, _ := item.ValueCopy(nil)
		lastBlock := DeserializeBlock(lastBlockData)

		if block.Height > lastBlock.Height {
			err = txn.Set([]byte("lh"), block.Hash)
			chain.LastHash = block.Hash
		}

		return err
	})
	if err != nil {
		log.Panic(err)
	}
	return true
}

// FindUnspentTransactions returns a list of transactions containing unspent outputs
func (chain *Blockchain) FindUnspentTransactions(pubKeyHash []byte) []Transaction {
	var unspentTXs []Transaction
	spentTXOs := make(map[string][]int)
	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Vout {
				// Was the output spent?
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] {
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}

				if out.IsLockedWithKey(pubKeyHash) {
					unspentTXs = append(unspentTXs, *tx)
				}
			}

			if !tx.IsCoinbase() {
				for _, in := range tx.Vin {
					if in.UsesKey(pubKeyHash) {
						inTxID := hex.EncodeToString(in.Txid)
						spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Vout)
					}
				}
			}
		}

		if len(block.PrevBlockHash) == 0 {
			break
		}
	}

	return unspentTXs
}

// FindTransactions searches for all transactions related to an address
func (chain *Blockchain) FindTransactions(address string) []Transaction {
	var txs []Transaction

	pubKeyHash, err := Base58Decode([]byte(address))
	if err != nil {
		fmt.Printf("Error decoding address: %s\n", err)
		return txs
	}
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4]

	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			// Check Outputs (Receiver)
			for _, out := range tx.Vout {
				if out.IsLockedWithKey(pubKeyHash) {
					txs = append(txs, *tx)
					goto NextTx // Avoid adding same tx twice
				}
			}

			// Check Inputs (Sender)
			if !tx.IsCoinbase() {
				for _, in := range tx.Vin {
					if in.UsesKey(pubKeyHash) {
						txs = append(txs, *tx)
						goto NextTx
					}
				}
			}

		NextTx:
		}

		if len(block.PrevBlockHash) == 0 {
			break
		}
	}
	return txs
}

// FindUTXO finds all unspent transaction outputs and returns them
func (chain *Blockchain) FindUTXO() map[string]TxOutputs {
	UTXO := make(map[string]TxOutputs)
	spentTXOs := make(map[string][]int)
	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Vout {
				// Was the output spent?
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] {
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}

				outs := UTXO[txID]
				outs.Outputs = append(outs.Outputs, out)
				UTXO[txID] = outs
			}

			if !tx.IsCoinbase() {
				for _, in := range tx.Vin {
					inTxID := hex.EncodeToString(in.Txid)
					spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Vout)
				}
			}
		}

		if len(block.PrevBlockHash) == 0 {
			break
		}
	}

	return UTXO
}

// FindSpendableOutputs finds and returns unspent outputs to reference in inputs
func (chain *Blockchain) FindSpendableOutputs(pubKeyHash []byte, amount int64) (int64, map[string][]int) {
	unspentOutputs := make(map[string][]int)
	accumulated := int64(0)
	unspentTXs := chain.FindUnspentTransactions(pubKeyHash)

Work:
	for _, tx := range unspentTXs {
		txID := hex.EncodeToString(tx.ID)

		for outIdx, out := range tx.Vout {
			if out.IsLockedWithKey(pubKeyHash) && accumulated < amount {
				accumulated += out.Value
				unspentOutputs[txID] = append(unspentOutputs[txID], outIdx)

				if accumulated >= amount {
					break Work
				}
			}
		}
	}

	return accumulated, unspentOutputs
}

// FindTransaction finds a transaction by ID
func (chain *Blockchain) FindTransaction(ID []byte) (Transaction, error) {
	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			if bytes.Equal(tx.ID, ID) {
				return *tx, nil
			}
		}

		if len(block.PrevBlockHash) == 0 {
			break
		}
	}

	return Transaction{}, errors.New("Transaction does not exist")
}

// SignTransaction signs inputs of a Transaction
func (chain *Blockchain) SignTransaction(tx *Transaction, privKey ecdsa.PrivateKey) {
	prevTXs := make(map[string]Transaction)

	for _, vin := range tx.Vin {
		prevTX, err := chain.FindTransaction(vin.Txid)
		if err != nil {
			log.Panic(err)
		}
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	tx.Sign(privKey, prevTXs)
}

// VerifyTransaction verifies transaction input signatures
func (chain *Blockchain) VerifyTransaction(tx *Transaction) bool {
	if tx.IsCoinbase() {
		return true
	}

	prevTXs := make(map[string]Transaction)

	for _, vin := range tx.Vin {
		prevTX, err := chain.FindTransaction(vin.Txid)
		if err != nil {
			log.Panic(err)
		}
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	return tx.Verify(prevTXs)
}

// Iterator returns a BlockchainIterator
func (chain *Blockchain) Iterator() *BlockchainIterator {
	iter := &BlockchainIterator{chain.LastHash, chain.Database}
	return iter
}

// Next returns the next block from the iterator
func (i *BlockchainIterator) Next() *Block {
	var block *Block

	err := i.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get(i.CurrentHash)
		if err != nil {
			log.Panic(err)
		}
		encodedBlock, err := item.ValueCopy(nil)
		block = DeserializeBlock(encodedBlock)
		return err
	})

	if err != nil {
		log.Panic(err)
	}

	i.CurrentHash = block.PrevBlockHash

	return block
}

// DeserializeBlock deserializes a block
func DeserializeBlock(d []byte) *Block {
	var block Block
	decoder := gob.NewDecoder(bytes.NewReader(d))
	err := decoder.Decode(&block)
	if err != nil {
		log.Panic(err)
	}
	return &block
}

func DBExists() bool {
	if _, err := os.Stat(dbPath + "/MANIFEST"); os.IsNotExist(err) {
		return false
	}
	return true
}
