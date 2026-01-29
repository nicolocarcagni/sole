package main

import (
	"encoding/hex"
	"log"

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
			key, err := hex.DecodeString(txId)
			if err != nil {
				return err
			}
			key = append(bucketName, key...)

			err = txn.Set(key, outs.Serialize())
			if err != nil {
				return err
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
					updatedOuts := TxOutputs{}
					inTxID := append([]byte(utxoPrefix), vin.Txid...)
					item, err := txn.Get(inTxID)
					if err == badger.ErrKeyNotFound {
						// Key missing: likely orphan block or double-spend attempt or re-processing.
						// We ignore it to prevent crash.
						// fmt.Printf("⚠️  [UTXO] Warning: Input %x not found (already spent?)\n", vin.Txid)
						continue
					} else if err != nil {
						return err
					}
					v, err := item.ValueCopy(nil)
					if err != nil {
						return err
					}

					outs := DeserializeOutputs(v)

					for outIdx, out := range outs.Outputs {
						if outIdx != vin.Vout {
							updatedOuts.Outputs = append(updatedOuts.Outputs, out)
						}
					}

					if len(updatedOuts.Outputs) == 0 {
						err := txn.Delete(inTxID)
						if err != nil {
							return err
						}
					} else {
						err := txn.Set(inTxID, updatedOuts.Serialize())
						if err != nil {
							return err
						}
					}
				}
			}

			newOutputs := TxOutputs{}
			for _, out := range tx.Vout {
				newOutputs.Outputs = append(newOutputs.Outputs, out)
			}

			txID := append([]byte(utxoPrefix), tx.ID...)
			err := txn.Set(txID, newOutputs.Serialize())
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		log.Panic(err)
	}
}

// FindSpendableOutputs finds and returns unspent outputs to reference in inputs
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
			k := item.Key()
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			k = k[len(utxoPrefix):]
			txID := hex.EncodeToString(k)
			outs := DeserializeOutputs(v)

			for outIdx, out := range outs.Outputs {
				if out.IsLockedWithKey(pubKeyHash) && accumulated < amount {
					accumulated += out.Value
					unspentOutputs[txID] = append(unspentOutputs[txID], outIdx)
				}
			}
		}
		return nil
	})
	if err != nil {
		log.Panic(err)
	}

	return accumulated, unspentOutputs
}

// FindUnspentTransactions returns a list of outputs belonging to the address
// Note: We return TxOutputs here since we don't need full Transaction struct for balance check
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
			outs := DeserializeOutputs(v)

			for _, out := range outs.Outputs {
				if out.IsLockedWithKey(pubKeyHash) {
					UTXOs = append(UTXOs, out)
				}
			}
		}
		return nil
	})
	if err != nil {
		log.Panic(err)
	}

	return UTXOs
}

// CountTransactions returns the number of transactions in the UTXO set
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
