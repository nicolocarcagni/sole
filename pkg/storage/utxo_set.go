package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/dgraph-io/badger/v3"

	"github.com/nicolocarcagni/sole/pkg/core"
)

const UtxoPrefix = "utxo-"

const (
	activeUTXOPrefixKey = "meta:utxo-active-prefix"
	utxoGenerationRoot  = "utxo-generation-"
)

type UTXOSet struct {
	Blockchain *Blockchain
}

func (u UTXOSet) Reindex() error {
	if u.Blockchain == nil || u.Blockchain.Database == nil {
		return errors.New("cannot rebuild UTXO set without an open blockchain database")
	}
	u.Blockchain.Mux.Lock()
	defer u.Blockchain.Mux.Unlock()

	var tipHash []byte
	if err := u.Blockchain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			return err
		}
		tipHash, err = item.ValueCopy(nil)
		return err
	}); err != nil {
		return fmt.Errorf("failed to read canonical tip for UTXO rebuild: %w", err)
	}
	prefix, err := u.stageReindex(tipHash, nil)
	if err != nil {
		if prefix != "" {
			u.cleanupUTXOPrefix(prefix)
		}
		return fmt.Errorf("failed to rebuild UTXO set: %w", err)
	}
	if err := u.Blockchain.Database.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(activeUTXOPrefixKey), []byte(prefix))
	}); err != nil {
		u.cleanupUTXOPrefix(prefix)
		return fmt.Errorf("failed to activate rebuilt UTXO set: %w", err)
	}
	u.cleanupInactiveUTXOPrefixes(prefix)
	return nil
}

func (u UTXOSet) Update(block *core.Block) error {
	if u.Blockchain == nil || u.Blockchain.Database == nil {
		return errors.New("cannot update UTXO set without an open blockchain database")
	}
	u.Blockchain.Mux.Lock()
	defer u.Blockchain.Mux.Unlock()
	err := u.Blockchain.Database.Update(func(txn *badger.Txn) error {
		return u.updateInTransaction(txn, block)
	})
	if err != nil {
		return fmt.Errorf("failed to update UTXO set: %w", err)
	}
	return nil
}

func (u UTXOSet) updateInTransaction(txn *badger.Txn, block *core.Block) error {
	prefix, err := activeUTXOPrefix(txn)
	if err != nil {
		return err
	}
	for _, tx := range block.Transactions {
		if !tx.IsCoinbase() {
			for _, vin := range tx.Vin {
				txID := hex.EncodeToString(vin.Txid)
				key := fmt.Sprintf("%s%s-%d", prefix, txID, vin.Vout)
				if err := txn.Delete([]byte(key)); err != nil && err != badger.ErrKeyNotFound {
					return err
				}
			}
		}
		for outIdx, out := range tx.Vout {
			if out.IsOPReturn() {
				continue
			}
			key := fmt.Sprintf("%s%s-%d", prefix, hex.EncodeToString(tx.ID), outIdx)
			serializedOut, err := SerializeUTXO(out)
			if err != nil {
				return err
			}
			if err := txn.Set([]byte(key), serializedOut); err != nil {
				return err
			}
		}
	}
	return nil
}

func (u UTXOSet) stageReindex(tipHash []byte, candidate *core.Block) (string, error) {
	utxos := make(map[string]map[int]core.TxOutput)
	spent := make(map[string]map[int]bool)
	currentHash := append([]byte(nil), tipHash...)
	if err := u.Blockchain.Database.View(func(txn *badger.Txn) error {
		for len(currentHash) != 0 {
			var block *core.Block
			if candidate != nil && bytes.Equal(currentHash, candidate.Hash) {
				block = candidate
			} else {
				item, err := txn.Get(currentHash)
				if err != nil {
					return fmt.Errorf("failed to read canonical block during UTXO rebuild: %w", err)
				}
				data, err := item.ValueCopy(nil)
				if err != nil {
					return err
				}
				block = core.DeserializeBlock(data)
				if block == nil {
					return errors.New("failed to decode canonical block during UTXO rebuild")
				}
			}
			for _, tx := range block.Transactions {
				txID := hex.EncodeToString(tx.ID)
				for outIdx, out := range tx.Vout {
					if spent[txID] != nil && spent[txID][outIdx] {
						continue
					}
					if utxos[txID] == nil {
						utxos[txID] = make(map[int]core.TxOutput)
					}
					utxos[txID][outIdx] = out
				}
				if !tx.IsCoinbase() {
					for _, input := range tx.Vin {
						inputID := hex.EncodeToString(input.Txid)
						if spent[inputID] == nil {
							spent[inputID] = make(map[int]bool)
						}
						spent[inputID][input.Vout] = true
					}
				}
			}
			currentHash = append(currentHash[:0], block.PrevBlockHash...)
		}
		return nil
	}); err != nil {
		return "", err
	}

	prefix, err := newUTXOGenerationPrefix()
	if err != nil {
		return "", err
	}
	batch := u.Blockchain.Database.NewWriteBatch()
	defer batch.Cancel()
	txIDs := make([]string, 0, len(utxos))
	for txID := range utxos {
		txIDs = append(txIDs, txID)
	}
	sort.Strings(txIDs)
	for _, txID := range txIDs {
		outputs := utxos[txID]
		outIndexes := make([]int, 0, len(outputs))
		for outIdx := range outputs {
			outIndexes = append(outIndexes, outIdx)
		}
		sort.Ints(outIndexes)
		for _, outIdx := range outIndexes {
			output := outputs[outIdx]
			if output.IsOPReturn() {
				continue
			}
			serialized, err := SerializeUTXO(output)
			if err != nil {
				return "", err
			}
			key := fmt.Sprintf("%s%s-%d", prefix, txID, outIdx)
			if err := batch.Set([]byte(key), serialized); err != nil {
				return prefix, err
			}
		}
	}
	if err := batch.Flush(); err != nil {
		return prefix, err
	}
	return prefix, nil
}

func newUTXOGenerationPrefix() (string, error) {
	var generation [16]byte
	if _, err := rand.Read(generation[:]); err != nil {
		return "", fmt.Errorf("failed to create UTXO generation identifier: %w", err)
	}
	return utxoGenerationRoot + hex.EncodeToString(generation[:]) + "-", nil
}

func activeUTXOPrefix(txn *badger.Txn) (string, error) {
	item, err := txn.Get([]byte(activeUTXOPrefixKey))
	if errors.Is(err, badger.ErrKeyNotFound) {
		return UtxoPrefix, nil
	}
	if err != nil {
		return "", err
	}
	prefix, err := item.ValueCopy(nil)
	if err != nil {
		return "", err
	}
	if len(prefix) == 0 {
		return "", errors.New("active UTXO prefix is empty")
	}
	activePrefix := string(prefix)
	if activePrefix != UtxoPrefix && (!strings.HasPrefix(activePrefix, utxoGenerationRoot) || !strings.HasSuffix(activePrefix, "-")) {
		return "", errors.New("active UTXO prefix has an invalid format")
	}
	return activePrefix, nil
}

func (u UTXOSet) cleanupInactiveUTXOPrefixes(activePrefix string) {
	batch := u.Blockchain.Database.NewWriteBatch()
	defer batch.Cancel()
	err := u.Blockchain.Database.View(func(txn *badger.Txn) error {
		options := badger.DefaultIteratorOptions
		options.Prefix = []byte(UtxoPrefix)
		iterator := txn.NewIterator(options)
		defer iterator.Close()
		for iterator.Rewind(); iterator.Valid(); iterator.Next() {
			key := iterator.Item().KeyCopy(nil)
			if bytes.HasPrefix(key, []byte(activePrefix)) {
				continue
			}
			if err := batch.Delete(key); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		err = batch.Flush()
	}
	if err != nil {
		fmt.Printf("warning: failed to remove inactive UTXO entries: %v\n", err)
	}
}

func (u UTXOSet) cleanupUTXOPrefix(prefix string) {
	if prefix == "" {
		return
	}
	batch := u.Blockchain.Database.NewWriteBatch()
	defer batch.Cancel()
	err := u.Blockchain.Database.View(func(txn *badger.Txn) error {
		options := badger.DefaultIteratorOptions
		options.Prefix = []byte(prefix)
		iterator := txn.NewIterator(options)
		defer iterator.Close()
		for iterator.Rewind(); iterator.Valid(); iterator.Next() {
			if err := batch.Delete(iterator.Item().KeyCopy(nil)); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		err = batch.Flush()
	}
	if err != nil {
		fmt.Printf("warning: failed to remove temporary UTXO entries: %v\n", err)
	}
}

func (u UTXOSet) FindSpendableOutputs(pubKeyHash []byte, amount int64) (int64, map[string][]int, error) {
	unspentOutputs := make(map[string][]int)
	accumulated := int64(0)
	db := u.Blockchain.Database

	err := db.View(func(txn *badger.Txn) error {
		prefix, err := activeUTXOPrefix(txn)
		if err != nil {
			return err
		}
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := string(item.Key())
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			// Active-prefix keys end with <txID>-<outIdx>.
			parts := strings.Split(strings.TrimPrefix(k, prefix), "-")
			if len(parts) < 2 {
				continue
			}
			txID := parts[0]
			outIdx, _ := strconv.Atoi(parts[1])

			out, err := DeserializeUTXO(v)
			if err != nil {
				return err
			}

			if out.IsLockedWithKey(pubKeyHash) && accumulated < amount {
				accumulated += out.Value
				unspentOutputs[txID] = append(unspentOutputs[txID], outIdx)
			}
		}
		return nil
	})
	if err != nil {
		return 0, nil, fmt.Errorf("failed to find spendable outputs: %w", err)
	}

	return accumulated, unspentOutputs, nil
}

func (u UTXOSet) FindUnspentOutputs(pubKeyHash []byte) ([]core.TxOutput, error) {
	var UTXOs []core.TxOutput
	db := u.Blockchain.Database

	err := db.View(func(txn *badger.Txn) error {
		prefix, err := activeUTXOPrefix(txn)
		if err != nil {
			return err
		}
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			out, err := DeserializeUTXO(v)
			if err != nil {
				return err
			}

			if out.IsLockedWithKey(pubKeyHash) {
				UTXOs = append(UTXOs, out)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find unspent outputs: %w", err)
	}

	return UTXOs, nil
}

type UTXO struct {
	TxID   string
	Vout   int
	Output core.TxOutput
}

func (u UTXOSet) FindAllUTXOs(pubKeyHash []byte) ([]UTXO, error) {
	var UTXOs []UTXO
	db := u.Blockchain.Database

	err := db.View(func(txn *badger.Txn) error {
		prefix, err := activeUTXOPrefix(txn)
		if err != nil {
			return err
		}
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := string(item.Key())
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			// Active-prefix keys end with <txID>-<outIdx>.
			parts := strings.Split(strings.TrimPrefix(k, prefix), "-")
			if len(parts) < 2 {
				continue
			}
			txID := parts[0]
			outIdx, _ := strconv.Atoi(parts[1])

			out, err := DeserializeUTXO(v)
			if err != nil {
				return err
			}

			if out.IsLockedWithKey(pubKeyHash) {
				UTXOs = append(UTXOs, UTXO{txID, outIdx, out})
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find unspent outputs: %w", err)
	}

	return UTXOs, nil
}

func (u UTXOSet) CountTransactions() (int, error) {
	db := u.Blockchain.Database
	counter := 0

	err := db.View(func(txn *badger.Txn) error {
		prefix, err := activeUTXOPrefix(txn)
		if err != nil {
			return err
		}
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			counter++
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count transactions: %w", err)
	}

	return counter, nil
}

// Helper functions for serialization since we are storing individual core.TxOutputs
func SerializeUTXO(out core.TxOutput) ([]byte, error) {
	var buff bytes.Buffer
	enc := gob.NewEncoder(&buff)
	err := enc.Encode(out)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize UTXO: %w", err)
	}
	return buff.Bytes(), nil
}

func DeserializeUTXO(data []byte) (core.TxOutput, error) {
	var out core.TxOutput
	dec := gob.NewDecoder(bytes.NewReader(data))
	err := dec.Decode(&out)
	if err != nil {
		return out, fmt.Errorf("failed to deserialize UTXO: %w", err)
	}
	return out, nil
}

func addAmount(total, value int64) (int64, error) {
	if total < 0 || value < 0 || value > math.MaxInt64-total {
		return 0, fmt.Errorf("amount total overflows int64")
	}
	return total + value, nil
}

func (u UTXOSet) ValidateBlockTransactions(block *core.Block) bool {
	db := u.Blockchain.Database
	valid := true

	// Keep track of outputs spent in this block to prevent double-spending
	spentInBlock := make(map[string]bool)
	// Keep track of outputs created in this block (mempool chaining)
	createdInBlock := make(map[string]bool)

	// Keep track of fees for Coinbase validation
	totalFees := int64(0)

	// ── Pass 1: Pre-populate block TX map and created outputs ────────────
	blockTxMap := make(map[string]*core.Transaction)
	for _, tx := range block.Transactions {
		if tx == nil {
			fmt.Println("⚠️ [ValidateBlockTransactions] Nil transaction found in block, rejecting...")
			return false
		}
		if err := tx.ValidateAmounts(); err != nil {
			fmt.Printf("⛔ [ValidateBlockTransactions] Invalid transaction amounts: %v\n", err)
			return false
		}
		txID := hex.EncodeToString(tx.ID)
		blockTxMap[txID] = tx
		for outIdx, out := range tx.Vout {
			if out.IsOPReturn() {
				continue
			}
			key := fmt.Sprintf("%s-%d", txID, outIdx)
			createdInBlock[key] = true
		}
	}

	// ── Pass 2: Validate inputs, fees, and double-spend rules ───────────
	err := db.View(func(txn *badger.Txn) error {
		prefix, err := activeUTXOPrefix(txn)
		if err != nil {
			return err
		}
		for _, tx := range block.Transactions {
			if tx.IsCoinbase() {
				continue
			}

			txInputTotal := int64(0)
			txOutputTotal := int64(0)

			for _, out := range tx.Vout {
				var addErr error
				txOutputTotal, addErr = addAmount(txOutputTotal, out.Value)
				if addErr != nil {
					valid = false
					return nil
				}
			}

			for _, vin := range tx.Vin {
				txID := hex.EncodeToString(vin.Txid)
				outpoint := fmt.Sprintf("%s-%d", txID, vin.Vout)
				key := prefix + outpoint

				// 1. Check if already spent by another transaction in THIS block
				if spentInBlock[outpoint] {
					fmt.Printf("⛔ [UTXOSet] Double spend detected within block! Input: %s\n", key)
					valid = false
					return nil
				}

				// 2. Check if the output was created in THIS block (intra-block spend)
				if createdInBlock[outpoint] {
					spentInBlock[outpoint] = true
					if parentTx, exists := blockTxMap[txID]; exists && parentTx != nil {
						if int(vin.Vout) < len(parentTx.Vout) {
							matchedOut := parentTx.Vout[vin.Vout]
							if !matchedOut.IsOPReturn() {
								var addErr error
								txInputTotal, addErr = addAmount(txInputTotal, matchedOut.Value)
								if addErr != nil {
									valid = false
									return nil
								}
							}
						}
					}
					continue
				}

				// 3. Otherwise, it MUST exist in the UTXO database
				item, err := txn.Get([]byte(key))
				if err == badger.ErrKeyNotFound {
					fmt.Printf("⛔ [UTXOSet] Invalid input! UTXO not found: %s\n", key)
					valid = false
					return nil
				} else if err != nil {
					return err
				}

				v, err := item.ValueCopy(nil)
				if err != nil {
					return err
				}
				out, err := DeserializeUTXO(v)
				if err != nil {
					return err
				}
				var addErr error
				txInputTotal, addErr = addAmount(txInputTotal, out.Value)
				if addErr != nil {
					valid = false
					return nil
				}

				spentInBlock[key] = true
			}

			if txInputTotal < txOutputTotal {
				fee := txInputTotal - txOutputTotal
				fmt.Printf("⛔ [UTXOSet] Invalid transaction: Fees cannot be negative (%d)\n", fee)
				valid = false
				return nil
			}
			fee := txInputTotal - txOutputTotal
			var addErr error
			totalFees, addErr = addAmount(totalFees, fee)
			if addErr != nil {
				valid = false
				return nil
			}
		}

		// Validate Coinbase core.Block Reward + Fees Limit
		if len(block.Transactions) > 0 && block.Transactions[0].IsCoinbase() {
			cbTx := block.Transactions[0]
			coinbaseValue := cbTx.Vout[0].Value
			allowedSubsidy := u.Blockchain.GetBlockSubsidy(block.Height)
			maxAllowedReward, addErr := addAmount(allowedSubsidy, totalFees)
			if addErr != nil {
				valid = false
				return nil
			}

			if coinbaseValue > maxAllowedReward {
				fmt.Printf("⛔ [UTXOSet] Invalid block: Coinbase reward %d exceeds max allowed %d (Subsidy: %d + Fees: %d)\n", coinbaseValue, maxAllowedReward, allowedSubsidy, totalFees)
				valid = false
				return nil
			}
		}

		return nil
	})

	if err != nil {
		fmt.Printf("⛔ [UTXOSet] Validation failed due to DB error: %s\n", err)
		return false
	}

	return valid
}

func (u UTXOSet) CalculateFee(tx *core.Transaction, mempool ...map[string]core.MempoolItem) (int64, error) {
	if tx == nil {
		return 0, fmt.Errorf("transaction is nil")
	}
	if err := tx.ValidateAmounts(); err != nil {
		return 0, fmt.Errorf("invalid transaction amounts: %w", err)
	}
	if tx.IsCoinbase() {
		return 0, nil
	}

	var mp map[string]core.MempoolItem
	if len(mempool) > 0 && mempool[0] != nil {
		mp = mempool[0]
	}

	var inputTotal int64
	var outputTotal int64

	for _, out := range tx.Vout {
		var err error
		outputTotal, err = addAmount(outputTotal, out.Value)
		if err != nil {
			return 0, err
		}
	}

	db := u.Blockchain.Database
	err := db.View(func(txn *badger.Txn) error {
		prefix, err := activeUTXOPrefix(txn)
		if err != nil {
			return err
		}
		for _, vin := range tx.Vin {
			txID := hex.EncodeToString(vin.Txid)
			key := fmt.Sprintf("%s%s-%d", prefix, txID, vin.Vout)

			item, err := txn.Get([]byte(key))
			if err == badger.ErrKeyNotFound {
				// Check mempool for unconfirmed parent transaction
				if mp != nil {
					if mempoolItem, exists := mp[txID]; exists {
						if int(vin.Vout) < len(mempoolItem.Tx.Vout) {
							parentOutput := mempoolItem.Tx.Vout[vin.Vout]
							var addErr error
							inputTotal, addErr = addAmount(inputTotal, parentOutput.Value)
							if addErr != nil {
								return addErr
							}
							continue
						}
					}
				}
				// Input is not in the UTXO set and not in the mempool.
				// The output either never existed or was already spent in a prior block.
				// Reject immediately to prevent the historical double-spend fee bypass.
				return fmt.Errorf("input outpoint %s:%d does not exist or has already been spent", txID, vin.Vout)
			} else if err != nil {
				return err
			}

			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			out, err := DeserializeUTXO(v)
			if err != nil {
				return err
			}
			inputTotal, err = addAmount(inputTotal, out.Value)
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return 0, err
	}

	if inputTotal < outputTotal {
		fee := inputTotal - outputTotal
		return 0, fmt.Errorf("negative fee (%d)", fee)
	}
	fee := inputTotal - outputTotal
	return fee, nil
}
