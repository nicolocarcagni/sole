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

var (
	ErrBlockchainNotFound = errors.New("blockchain database does not exist")
	ErrBlockchainExists   = errors.New("blockchain database already exists")
	ErrBlockNotFound      = errors.New("block not found")
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

type Blockchain struct {
	LastHash []byte
	Database *badger.DB
	Mux      sync.Mutex
}

type BlockchainIterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

func InitBlockchain() (*Blockchain, error) {
	var lastHash []byte

	if DBExists() {
		return nil, ErrBlockchainExists
	}

	// Ensure data directory exists
	if err := os.MkdirAll(dbPath, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %s", err)
	}

	opts := getBadgerOptions(dbPath)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	err = db.Update(func(txn *badger.Txn) error {
		genesis, err := NewGenesisBlock()
		if err != nil {
			return err
		}
		fmt.Println("🌟 Genesis Block created")

		serialized, err := genesis.Serialize()
		if err != nil {
			return err
		}
		err = txn.Set(genesis.Hash, serialized)
		if err != nil {
			return fmt.Errorf("failed to save genesis block: %w", err)
		}

		// [OPTIMIZATION] Index transactions for O(1) lookup
		for _, tx := range genesis.Transactions {
			err = txn.Set(append([]byte("tx-"), tx.ID...), genesis.Hash)
			if err != nil {
				return fmt.Errorf("failed to index genesis transactions: %w", err)
			}
		}

		err = txn.Set([]byte("lh"), genesis.Hash)
		lastHash = genesis.Hash
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("InitBlockchain database update failed: %w", err)
	}

	blockchain := Blockchain{lastHash, db, sync.Mutex{}}
	return &blockchain, nil
}

func ContinueBlockchain(address string) (*Blockchain, error) {
	if !DBExists() {
		return nil, ErrBlockchainNotFound
	}

	var lastHash []byte
	opts := badger.DefaultOptions(dbPath)
	opts.Logger = nil

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	err = db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			return err
		}
		lastHash, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve last block hash: %w", err)
	}

	chain := Blockchain{lastHash, db, sync.Mutex{}}
	return &chain, nil
}

func ContinueBlockchainReadOnly(address string) (*Blockchain, error) {
	if !DBExists() {
		return nil, ErrBlockchainNotFound
	}

	var lastHash []byte
	opts := badger.DefaultOptions(dbPath)
	opts.Logger = nil
	opts.ReadOnly = true

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	err = db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			return err
		}
		lastHash, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve last block hash (Read-Only): %w", err)
	}

	chain := Blockchain{lastHash, db, sync.Mutex{}}
	return &chain, nil
}

func ContinueBlockchainSnapshot(customPath string) (*Blockchain, error) {
	if _, err := os.Stat(customPath + "/MANIFEST"); os.IsNotExist(err) {
		return nil, fmt.Errorf("snapshot DB corrupt or missing: %w", err)
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
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	err = db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			return err
		}
		lastHash, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve last block hash: %w", err)
	}

	chain := Blockchain{lastHash, db, sync.Mutex{}}
	return &chain, nil
}

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
// Returns hashes in chronological order: Genesis → Tip
func (chain *Blockchain) GetBlockHashes() [][]byte {
	var blocks [][]byte

	iter := chain.Iterator()

	for {
		block, err := iter.Next()
		if err != nil {
			break
		}
		blocks = append(blocks, block.Hash)

		if len(block.PrevBlockHash) == 0 {
			break
		}
	}

	// Reverse: iterator walks tip→genesis, but we need genesis→tip for IBD
	for i, j := 0, len(blocks)-1; i < j; i, j = i+1, j-1 {
		blocks[i], blocks[j] = blocks[j], blocks[i]
	}

	return blocks
}

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

func (chain *Blockchain) ForgeBlock(transactions []*Transaction, privKey ecdsa.PrivateKey) (*Block, error) {
	chain.Mux.Lock()
	defer chain.Mux.Unlock()

	var lastHash []byte

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			return err
		}
		lastHash, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve last hash during forgery: %w", err)
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
		return nil, fmt.Errorf("failed to retrieve last block data: %w", err)
	}

	lastBlock := DeserializeBlock(lastBlockData)
	newHeight := lastBlock.Height + 1

	// Derive the validator's 64-byte raw public key (X‖Y, 32 bytes each) BEFORE
	// constructing the block so that the Validator field is bound into the header
	// prior to hashing and mining. This ensures block.Hash commits to the Validator.
	valPubKey := append(privKey.PublicKey.X.FillBytes(make([]byte, 32)),
		privKey.PublicKey.Y.FillBytes(make([]byte, 32))...)

	// Create block with Validator already set so the initial hash includes it.
	newBlock := NewBlock(transactions, lastHash, newHeight, valPubKey)

	// PoA Hardening: Mine the block (Find valid Nonce).
	// The mining loop hashes the header which already contains the Validator.
	MineBlock(newBlock)

	// Sign the block. SignBlock will verify Validator matches privKey and that
	// block.Hash == block.CalculateHash() before signing.
	err = SignBlock(newBlock, privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign block: %w", err)
	}

	err = chain.Database.Update(func(txn *badger.Txn) error {
		serialized, err := newBlock.Serialize()
		if err != nil {
			return err
		}
		err = txn.Set(newBlock.Hash, serialized)
		if err != nil {
			return err
		}

		// [OPTIMIZATION] Index transactions for O(1) lookup
		for _, tx := range newBlock.Transactions {
			err = txn.Set(append([]byte("tx-"), tx.ID...), newBlock.Hash)
			if err != nil {
				return err
			}
		}

		err = txn.Set([]byte("lh"), newBlock.Hash)
		if err != nil {
			return err
		}
		chain.LastHash = newBlock.Hash
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update database during forgery: %w", err)
	}

	return newBlock, nil
}

func (chain *Blockchain) AddBlock(block *Block, txCache ...map[string]Transaction) bool {
	// 0. Exist Check: Verify duplicates BEFORE expensive crypto validation
	_, err := chain.GetBlock(block.Hash)
	if err == nil {
		return false // Already processed
	}

	// 1. PoA Hardening: Validate block linkage and header
	chain.Mux.Lock()
	defer chain.Mux.Unlock()

	if len(block.PrevBlockHash) > 0 {
		var prevBlock Block
		err = chain.Database.View(func(txn *badger.Txn) error {
			item, err := txn.Get(block.PrevBlockHash)
			if err != nil {
				return err
			}
			data, _ := item.ValueCopy(nil)
			prevBlock = *DeserializeBlock(data)
			return nil
		})
		if err != nil {
			fmt.Printf("⛔ AddBlock: Parent block %x not found in DB. Orphan rejected.\n", block.PrevBlockHash[:4])
			return false
		}

		if block.Height != prevBlock.Height+1 {
			fmt.Printf("⛔ AddBlock: Height mismatch. Expected %d, got %d\n", prevBlock.Height+1, block.Height)
			return false
		}

		if err := ValidateBlockHeader(block, &prevBlock); err != nil {
			fmt.Printf("⛔ AddBlock: Header Validation Failed: %s\n", err)
			return false
		}
	}

	// 2. Strict Hash Integrity — verify block.Hash equals the recomputed hash of the
	// current header. This prevents header malleability attacks where a peer sends a
	// block with tampered fields (Validator, transactions, etc.) and a fake Hash that
	// happens to start with 0x00 (passing the raw PoW check).
	if !bytes.Equal(block.Hash, block.CalculateHash()) {
		fmt.Printf("⛔ AddBlock: Block rejected — hash integrity check failed for block at height %d\n", block.Height)
		return false
	}

	// 3. Verify PoA signature
	if !VerifyBlockSignature(block) {
		fmt.Println("AddBlock: Block rejected - invalid PoA signature")
		return false
	}

	// 4. Verify all internal transaction signatures (including intra-block + cross-block cache)
	if !chain.VerifyBlockTransactions(block, txCache...) {
		fmt.Println("AddBlock: Block rejected - invalid transaction signatures")
		return false
	}

	err = chain.Database.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(block.Hash); err == nil {
			return nil
		}

		blockData, err := block.Serialize()
		if err != nil {
			return err
		}
		err = txn.Set(block.Hash, blockData)
		if err != nil {
			return err
		}

		// Index transactions for O(1) lookup
		for _, tx := range block.Transactions {
			err = txn.Set(append([]byte("tx-"), tx.ID...), block.Hash)
			if err != nil {
				return err
			}
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
		fmt.Printf("⛔ AddBlock: Failed to save block to database: %v\n", err)
		return false
	}
	return true
}

const (
	MaxSupply       = 8910000 * 100000000 // 8.91M * 10^8
	InitialSubsidy  = 10 * 100000000      // 10 SOLE
	HalvingInterval = 195500              // Blocks
)

// GetBlockSubsidy calculates the mining reward based on block height (Halving)
func (chain *Blockchain) GetBlockSubsidy(height int) int64 {
	halvings := height / HalvingInterval

	// Safety check: If halvings >= 64, shifting will overflow/zero out anyway
	if halvings >= 64 {
		return 0
	}

	subsidy := int64(InitialSubsidy) >> halvings

	if subsidy <= 0 {
		return 0
	}

	return subsidy
}

// FindUnspentTransactions returns a list of transactions containing unspent outputs
func (bc *Blockchain) FindUnspentTransactions(pubKeyHash []byte) []Transaction {
	var unspentTXs []Transaction
	spentTXOs := make(map[string][]int)
	iter := bc.Iterator()

	for {
		block, err := iter.Next()
		if err != nil {
			break
		}

		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Vout {
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

			if tx.IsCoinbase() == false {
				for _, vin := range tx.Vin {
					inTxID := hex.EncodeToString(vin.Txid)
					spentTXOs[inTxID] = append(spentTXOs[inTxID], vin.Vout)
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
func (bc *Blockchain) FindTransactions(address string) []Transaction {
	var transactions []Transaction
	pubKeyHash, err := ExtractPubKeyHash(address)
	if err != nil {
		return transactions
	}
	iter := bc.Iterator()

	for {
		block, err := iter.Next()
		if err != nil {
			break
		}

		for _, tx := range block.Transactions {
			if tx.IsCoinbase() {
				for _, out := range tx.Vout {
					if out.IsLockedWithKey(pubKeyHash) {
						transactions = append(transactions, *tx)
						break
					}
				}
			} else {
				isRelated := false
				for _, vin := range tx.Vin {
					if bytes.Equal(HashPubKey(vin.PubKey), pubKeyHash) {
						isRelated = true
						break
					}
				}
				if !isRelated {
					for _, out := range tx.Vout {
						if out.IsLockedWithKey(pubKeyHash) {
							isRelated = true
							break
						}
					}
				}

				if isRelated {
					transactions = append(transactions, *tx)
				}
			}
		}

		if len(block.PrevBlockHash) == 0 {
			break
		}
	}

	return transactions
}

// FindUTXO finds all unspent transaction outputs and returns them.
// The returned map is keyed by hex-encoded transaction ID; the inner map is
// keyed by the original Vout index from the transaction, preserving the true
// output position so that downstream consumers (Reindex, FindSpendableOutputs)
// write and read the correct utxo-<txID>-<vout> keys in BadgerDB.
func (chain *Blockchain) FindUTXO() map[string]map[int]TxOutput {
	UTXO := make(map[string]map[int]TxOutput)
	// Use a nested map for O(1) spent-output lookup instead of O(n) slice scan.
	spentTXOs := make(map[string]map[int]bool)
	iter := chain.Iterator()

	for {
		block, err := iter.Next()
		if err != nil {
			break
		}

		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

			for outIdx, out := range tx.Vout {
				// Was the output spent? O(1) map lookup.
				if spentTXOs[txID] != nil && spentTXOs[txID][outIdx] {
					continue
				}

				if UTXO[txID] == nil {
					UTXO[txID] = make(map[int]TxOutput)
				}
				// Key is the ORIGINAL Vout index — never the compacted slice position.
				UTXO[txID][outIdx] = out
			}

			if !tx.IsCoinbase() {
				for _, in := range tx.Vin {
					inTxID := hex.EncodeToString(in.Txid)
					if spentTXOs[inTxID] == nil {
						spentTXOs[inTxID] = make(map[int]bool)
					}
					spentTXOs[inTxID][in.Vout] = true
				}
			}
		}

		if len(block.PrevBlockHash) == 0 {
			break
		}
	}

	return UTXO
}

// FindSpendableOutputs finds and returns unspent outputs to reference in inputs.
// It uses FindUTXO() directly to guarantee only true UTXOs are iterated and
// that returned indices match the canonical Vout positions in the transaction.
func (chain *Blockchain) FindSpendableOutputs(pubKeyHash []byte, amount int64) (int64, map[string][]int) {
	unspentOutputs := make(map[string][]int)
	accumulated := int64(0)
	utxos := chain.FindUTXO()

Work:
	for txID, outs := range utxos {
		for outIdx, out := range outs {
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

// FindTransaction finds a transaction by ID (Optimized with O(1) Index)
func (chain *Blockchain) FindTransaction(ID []byte) (Transaction, error) {
	// 1. Try to find using the O(1) Transaction Index
	var blockHash []byte
	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get(append([]byte("tx-"), ID...))
		if err != nil {
			return err
		}
		blockHash, err = item.ValueCopy(nil)
		return err
	})

	if err == nil {
		// Index hit! Retrieve the specific block
		block, err := chain.GetBlock(blockHash)
		if err == nil {
			for _, tx := range block.Transactions {
				if bytes.Equal(tx.ID, ID) {
					return *tx, nil
				}
			}
		}
	}

	// 2. Fallback to O(N) iteration (for legacy compatibility if DB not reset)
	iter := chain.Iterator()
	for {
		block, err := iter.Next()
		if err != nil {
			break
		}

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
func (chain *Blockchain) SignTransaction(tx *Transaction, privKey ecdsa.PrivateKey) error {
	prevTXs := make(map[string]Transaction)

	for _, vin := range tx.Vin {
		prevTX, err := chain.FindTransaction(vin.Txid)
		if err != nil {
			// [SECURITY FIX] Do not panic on invalid TxID, prevent DoS.
			fmt.Printf("⚠️  [SignTransaction] Skipped: Previous transaction not found (%x)\n", vin.Txid)
			return err
		}
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	return tx.Sign(privKey, prevTXs)
}

// VerifyTransaction verifies transaction input signatures (DB-only lookup)
func (chain *Blockchain) VerifyTransaction(tx *Transaction) bool {
	if tx.IsCoinbase() {
		return true
	}

	prevTXs := make(map[string]Transaction)

	for _, vin := range tx.Vin {
		prevTX, err := chain.FindTransaction(vin.Txid)
		if err != nil {
			fmt.Printf("⛔ [VerifyTransaction] Rejected: Parent transaction %x does not exist.\n", vin.Txid)
			return false
		}
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	return tx.Verify(prevTXs)
}

// FindTransactionWithMempool checks the mempool first, then falls back to the blockchain DB.
func (chain *Blockchain) FindTransactionWithMempool(ID []byte, mempool map[string]MempoolItem) (Transaction, error) {
	txID := hex.EncodeToString(ID)
	if item, exists := mempool[txID]; exists {
		return item.Tx, nil
	}
	return chain.FindTransaction(ID)
}

// VerifyTransactionWithMempool verifies transaction input signatures,
// checking the mempool for unconfirmed parent transactions before the DB.
func (chain *Blockchain) VerifyTransactionWithMempool(tx *Transaction, mempool map[string]MempoolItem) bool {
	if tx.IsCoinbase() {
		return true
	}

	prevTXs := make(map[string]Transaction)

	for _, vin := range tx.Vin {
		prevTX, err := chain.FindTransactionWithMempool(vin.Txid, mempool)
		if err != nil {
			fmt.Printf("⛔ [VerifyTransaction] Rejected: Parent transaction %x not found in DB or Mempool.\n", vin.Txid)
			return false
		}
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	return tx.Verify(prevTXs)
}

// VerifyBlockTransactions validates all transaction signatures in a block
// using a two-pass approach to handle arbitrary intra-block TX ordering.
// The optional externalCache accumulates TXs across blocks during IBD.
func (chain *Blockchain) VerifyBlockTransactions(block *Block, externalCache ...map[string]Transaction) bool {
	// Extract optional external (cross-block IBD) cache
	var crossBlockCache map[string]Transaction
	if len(externalCache) > 0 && externalCache[0] != nil {
		crossBlockCache = externalCache[0]
	}

	// ── Pass 1: Pre-populate block TX cache ─────────────────────────────
	blockTxCache := make(map[string]Transaction)

	// Merge cross-block IBD cache first (lower priority)
	for k, v := range crossBlockCache {
		blockTxCache[k] = v
	}

	// Then overlay this block's own TXs (higher priority)
	for _, tx := range block.Transactions {
		if tx == nil {
			log.Println("⚠️ [VerifyBlockTransactions] Nil transaction found in block, rejecting...")
			return false
		}
		blockTxCache[hex.EncodeToString(tx.ID)] = *tx
	}

	// ── Pass 2: Validate each transaction with the pre-populated cache ──
	for _, tx := range block.Transactions {
		if tx.IsCoinbase() {
			continue
		}

		prevTXs := make(map[string]Transaction)
		for _, vin := range tx.Vin {
			parentTxID := hex.EncodeToString(vin.Txid)

			if cachedTx, exists := blockTxCache[parentTxID]; exists {
				prevTXs[parentTxID] = cachedTx
			} else {
				// Fallback to blockchain database
				prevTX, err := chain.FindTransaction(vin.Txid)
				if err != nil {
					fmt.Printf("⛔ [VerifyBlockTransactions] Rejected: Parent transaction %x not found.\n", vin.Txid)
					return false
				}
				prevTXs[parentTxID] = prevTX
			}
		}

		if !tx.Verify(prevTXs) {
			fmt.Printf("⛔ [VerifyBlockTransactions] Rejected: Invalid signature in transaction %x\n", tx.ID)
			return false
		}
	}

	// ── Post-verification: feed this block's TXs into the IBD cache ─────
	if crossBlockCache != nil {
		for _, tx := range block.Transactions {
			crossBlockCache[hex.EncodeToString(tx.ID)] = *tx
		}
	}

	return true
}

// Iterator returns a BlockchainIterator
func (chain *Blockchain) Iterator() *BlockchainIterator {
	iter := &BlockchainIterator{chain.LastHash, chain.Database}
	return iter
}

// Next returns the next block from the iterator
func (i *BlockchainIterator) Next() (*Block, error) {
	var block *Block

	err := i.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get(i.CurrentHash)
		if err != nil {
			return err
		}
		encodedBlock, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		block = DeserializeBlock(encodedBlock)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get block from iterator: %w", err)
	}

	if block != nil {
		i.CurrentHash = block.PrevBlockHash
	}

	return block, nil
}

// DeserializeBlock deserializes a block
func DeserializeBlock(d []byte) *Block {
	var block Block
	decoder := gob.NewDecoder(bytes.NewReader(d))
	err := decoder.Decode(&block)
	if err != nil {
		log.Printf("⚠️ DeserializeBlock failed (%d bytes): %v", len(d), err)
		return nil
	}
	return &block
}

func DBExists() bool {
	if _, err := os.Stat(dbPath + "/MANIFEST"); os.IsNotExist(err) {
		return false
	}
	return true
}
