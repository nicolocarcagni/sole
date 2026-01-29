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
		log.Panic(err)
	}

	UTXO := u.Blockchain.FindUTXO()

	err = db.Update(func(txn *badger.Txn) error {
		for txId, outs := range UTXO {
			for outIdx, out := range outs.Outputs {
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
		log.Panic(err)
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
