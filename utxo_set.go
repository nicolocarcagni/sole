package main

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/dgraph-io/badger/v3"
)

const utxoPrefix = "utxo-"

// UTXOSet represents the UTXO set
type UTXOSet struct {
	Blockchain *Blockchain
}

// Reindex rebuilds the UTXO set
func (u UTXOSet) Reindex() {
	db := u.Blockchain.Database
	bucketName := []byte(utxoPrefix)

	err := db.Update(func(txn *badger.Txn) error {
		err := db.DropPrefix(bucketName)
		return err
	})
	if err != nil {
		log.Fatalf("Fatal: Failed to clear UTXO set prefix: %v", err)
	}

	UTXO := u.Blockchain.FindUTXO()

	err = db.Update(func(txn *badger.Txn) error {
		for txId, outs := range UTXO {
			for outIdx, out := range outs.Outputs {
				if out.IsOPReturn() {
					continue
				}
				key := fmt.Sprintf("%s%s-%d", utxoPrefix, txId, outIdx)
				err := txn.Set([]byte(key), SerializeUTXO(out))
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Fatal: Failed to rebuild UTXO set: %v", err)
	}
}

// Update updates the UTXO set with transactions from the Block
// The Block must be considered "newly added" (tip).
func (u UTXOSet) Update(block *Block) {
	db := u.Blockchain.Database

	err := db.Update(func(txn *badger.Txn) error {
		for _, tx := range block.Transactions {
			if !tx.IsCoinbase() {
				for _, vin := range tx.Vin {
					txID := hex.EncodeToString(vin.Txid)
					key := fmt.Sprintf("%s%s-%d", utxoPrefix, txID, vin.Vout)

					// Delete spent output
					err := txn.Delete([]byte(key))
					if err == badger.ErrKeyNotFound {
						// Ignored to prevent crash on re-org or double-spend attempt
					} else if err != nil {
						return err
					}
				}
			}

			// Add new outputs
			for outIdx, out := range tx.Vout {
				if out.IsOPReturn() {
					continue
				}
				txID := hex.EncodeToString(tx.ID)
				key := fmt.Sprintf("%s%s-%d", utxoPrefix, txID, outIdx)

				err := txn.Set([]byte(key), SerializeUTXO(out))
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		log.Panic(err)
	}
}

// FindSpendableOutputs finds and returns unspent outputs to reference in inputs
// Returns accumulated amount and a map of TxID -> []Vout (Output Index)
func (u UTXOSet) FindSpendableOutputs(pubKeyHash []byte, amount int64) (int64, map[string][]int) {
	unspentOutputs := make(map[string][]int)
	accumulated := int64(0)
	db := u.Blockchain.Database

	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(utxoPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := string(item.Key())
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			// Key format: utxo-<txID>-<outIdx>
			parts := strings.Split(k, "-")
			if len(parts) < 3 {
				continue
			}
			txID := parts[1]
			outIdx, _ := strconv.Atoi(parts[2])

			out := DeserializeUTXO(v)

			if out.IsLockedWithKey(pubKeyHash) && accumulated < amount {
				accumulated += out.Value
				unspentOutputs[txID] = append(unspentOutputs[txID], outIdx)
			}
		}
		return nil
	})
	if err != nil {
		log.Panic(err)
	}

	return accumulated, unspentOutputs
}

// FindUnspentOutputs returns a list of outputs belonging to the address
// Used for Balance calculation
func (u UTXOSet) FindUnspentOutputs(pubKeyHash []byte) []TxOutput {
	var UTXOs []TxOutput
	db := u.Blockchain.Database

	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(utxoPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			out := DeserializeUTXO(v)

			if out.IsLockedWithKey(pubKeyHash) {
				UTXOs = append(UTXOs, out)
			}
		}
		return nil
	})
	if err != nil {
		log.Panic(err)
	}

	return UTXOs
}

// CountTransactions returns the number of UTXOs (not Transactions!)
func (u UTXOSet) CountTransactions() int {
	db := u.Blockchain.Database
	counter := 0

	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(utxoPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			counter++
		}
		return nil
	})
	if err != nil {
		log.Panic(err)
	}

	return counter
}

// Helper functions for serialization since we are storing individual TxOutputs
func SerializeUTXO(out TxOutput) []byte {
	var buff bytes.Buffer
	enc := gob.NewEncoder(&buff)
	err := enc.Encode(out)
	if err != nil {
		log.Panic(err)
	}
	return buff.Bytes()
}

func DeserializeUTXO(data []byte) TxOutput {
	var out TxOutput
	dec := gob.NewDecoder(bytes.NewReader(data))
	err := dec.Decode(&out)
	if err != nil {
		log.Panic(err)
	}
	return out
}

// ValidateBlockTransactions verifies that all transactions in a block
// only spend unspent outputs, including strict checks against double-spending
// within the exact same block (mempool chaining).
func (u UTXOSet) ValidateBlockTransactions(block *Block) bool {
	db := u.Blockchain.Database
	valid := true

	// Keep track of outputs spent in this block to prevent double-spending
	spentInBlock := make(map[string]bool)
	// Keep track of outputs created in this block (mempool chaining)
	createdInBlock := make(map[string]bool)

	// Keep track of fees for Coinbase validation
	totalFees := int64(0)

	// First pass: Record all standard outputs created in this block
	for _, tx := range block.Transactions {
		txID := hex.EncodeToString(tx.ID)
		for outIdx, out := range tx.Vout {
			if out.IsOPReturn() {
				continue
			}
			key := fmt.Sprintf("%s%s-%d", utxoPrefix, txID, outIdx)
			createdInBlock[key] = true
		}
	}

	err := db.View(func(txn *badger.Txn) error {
		for _, tx := range block.Transactions {
			if tx.IsCoinbase() {
				continue
			}
			
			txInputTotal := int64(0)
			txOutputTotal := int64(0)

			// Calculate outputs total
			for _, out := range tx.Vout {
				txOutputTotal += out.Value
			}

			for _, vin := range tx.Vin {
				txID := hex.EncodeToString(vin.Txid)
				key := fmt.Sprintf("%s%s-%d", utxoPrefix, txID, vin.Vout)

				// 1. Check if already spent by another transaction in THIS block
				if spentInBlock[key] {
					fmt.Printf("⛔ [UTXOSet] Double spend detected within block! Input: %s\n", key)
					valid = false
					return nil // Stop checking
				}

				// 2. Check if the output was created in THIS block
				if createdInBlock[key] {
					spentInBlock[key] = true
					// For fee calculation, find the output value in the tx that created it
					// We find the tx within this block
					for _, intraTx := range block.Transactions {
						if hex.EncodeToString(intraTx.ID) == txID {
							matchedOut := intraTx.Vout[vin.Vout]
							if !matchedOut.IsOPReturn() {
								txInputTotal += matchedOut.Value
							}
							break
						}
					}
					continue // Valid intra-block spend
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

				// Read output to get the input value for fee calculation
				v, err := item.ValueCopy(nil)
				if err != nil {
					return err
				}
				out := DeserializeUTXO(v)
				txInputTotal += out.Value

				// Mark as spent to prevent double-spending in subsequent transactions in this block
				spentInBlock[key] = true
			}

			// Add to total block fees
			fee := txInputTotal - txOutputTotal
			if fee < 0 {
				fmt.Printf("⛔ [UTXOSet] Invalid transaction: Fees cannot be negative (%d)\n", fee)
				valid = false
				return nil
			}
			totalFees += fee
		}

		// 4. Validate Coinbase Block Reward + Fees Limit
		// Ensure that the coinbase output does not exceed the allowed subsidy + collected fees
		if len(block.Transactions) > 0 && block.Transactions[0].IsCoinbase() {
			cbTx := block.Transactions[0]
			coinbaseValue := cbTx.Vout[0].Value
			allowedSubsidy := u.Blockchain.GetBlockSubsidy(block.Height)
			maxAllowedReward := allowedSubsidy + totalFees

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

// CalculateFee calculates the implicit fee of a transaction: Sum(Inputs) - Sum(Outputs)
func (u UTXOSet) CalculateFee(tx *Transaction) (int64, error) {
	if tx.IsCoinbase() {
		return 0, nil
	}

	var inputTotal int64
	var outputTotal int64

	for _, out := range tx.Vout {
		outputTotal += out.Value
	}

	db := u.Blockchain.Database
	err := db.View(func(txn *badger.Txn) error {
		for _, vin := range tx.Vin {
			txID := hex.EncodeToString(vin.Txid)
			key := fmt.Sprintf("%s%s-%d", utxoPrefix, txID, vin.Vout)

			item, err := txn.Get([]byte(key))
			if err == badger.ErrKeyNotFound {
				// Mempool chaining: the input might not be in DB yet if it's spending an unconfirmed tx.
				// In CalculateFee (used primarily before mining/mempool), we might get here.
				// We fall back to the Blockchain context to find it.
				prevTx, err := u.Blockchain.FindTransaction(vin.Txid)
				if err != nil {
					return fmt.Errorf("input tx %s not found in DB or Mempool", txID)
				}
				inputTotal += prevTx.Vout[vin.Vout].Value
				continue
			} else if err != nil {
				return err
			}

			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			out := DeserializeUTXO(v)
			inputTotal += out.Value
		}
		return nil
	})

	if err != nil {
		return 0, err
	}

	fee := inputTotal - outputTotal
	return fee, nil
}
