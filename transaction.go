package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"
)

// TxOutput represents a transaction output
type TxOutput struct {
	Value      int64
	PubKeyHash []byte
}

// Lock signs the output
func (out *TxOutput) Lock(address []byte) {
	pubKeyHash, err := Base58Decode(address)
	if err != nil {
		log.Panic(err)
	}
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4]
	out.PubKeyHash = pubKeyHash
}

// IsLockedWithKey checks if the output can be used by the owner of the pubkey
func (out *TxOutput) IsLockedWithKey(pubKeyHash []byte) bool {
	return bytes.Equal(out.PubKeyHash, pubKeyHash)
}

// NewTxOutput creates a new TXOutput
func NewTxOutput(value int64, address string) *TxOutput {
	txo := &TxOutput{value, nil}
	txo.Lock([]byte(address))
	return txo
}

// TxOutputs collects TxOutput
type TxOutputs struct {
	Outputs []TxOutput
}

// Serialize serializes TxOutputs
func (outs TxOutputs) Serialize() []byte {
	var buff bytes.Buffer
	enc := gob.NewEncoder(&buff)
	err := enc.Encode(outs)
	if err != nil {
		log.Panic(err)
	}
	return buff.Bytes()
}

// DeserializeOutputs deserializes TxOutputs
func DeserializeOutputs(data []byte) TxOutputs {
	var outputs TxOutputs
	dec := gob.NewDecoder(bytes.NewReader(data))
	err := dec.Decode(&outputs)
	if err != nil {
		log.Panic(err)
	}
	return outputs
}

// TxInput represents a transaction input
type TxInput struct {
	Txid      []byte
	Vout      int
	Signature []byte
	PubKey    []byte
}

// UsesKey checks whether the address initiated the transaction
func (in *TxInput) UsesKey(pubKeyHash []byte) bool {
	lockingHash := HashPubKey(in.PubKey)
	return bytes.Equal(lockingHash, pubKeyHash)
}

// Transaction represents a Bitcoin-like transaction
type Transaction struct {
	ID        []byte
	Vin       []TxInput
	Vout      []TxOutput
	Timestamp int64
}

// Serialize returns a serialized Transaction (Manual Binary Encoding for Interop)
func (tx Transaction) Serialize() []byte {
	var encoded bytes.Buffer

	// Inputs
	binary.Write(&encoded, binary.BigEndian, int64(len(tx.Vin)))
	for _, vin := range tx.Vin {
		binary.Write(&encoded, binary.BigEndian, int64(len(vin.Txid)))
		encoded.Write(vin.Txid)
		binary.Write(&encoded, binary.BigEndian, int64(vin.Vout))
		binary.Write(&encoded, binary.BigEndian, int64(len(vin.Signature)))
		encoded.Write(vin.Signature)
		binary.Write(&encoded, binary.BigEndian, int64(len(vin.PubKey)))
		encoded.Write(vin.PubKey)
	}

	// Outputs
	binary.Write(&encoded, binary.BigEndian, int64(len(tx.Vout)))
	for _, vout := range tx.Vout {
		binary.Write(&encoded, binary.BigEndian, vout.Value)
		binary.Write(&encoded, binary.BigEndian, int64(len(vout.PubKeyHash)))
		encoded.Write(vout.PubKeyHash)
	}

	// Timestamp
	binary.Write(&encoded, binary.BigEndian, tx.Timestamp)

	return encoded.Bytes()
}

// DeserializeTransaction decodes a transaction from bytes
func DeserializeTransaction(data []byte) Transaction {
	var tx Transaction
	reader := bytes.NewReader(data)

	// Inputs
	var inputsCount int64
	binary.Read(reader, binary.BigEndian, &inputsCount)
	for i := 0; i < int(inputsCount); i++ {
		var vin TxInput
		var lenVal int64

		// TxID
		binary.Read(reader, binary.BigEndian, &lenVal)
		vin.Txid = make([]byte, lenVal)
		reader.Read(vin.Txid)

		// Vout
		var vout int64
		binary.Read(reader, binary.BigEndian, &vout)
		vin.Vout = int(vout)

		// Signature
		binary.Read(reader, binary.BigEndian, &lenVal)
		vin.Signature = make([]byte, lenVal)
		reader.Read(vin.Signature)

		// PubKey
		binary.Read(reader, binary.BigEndian, &lenVal)
		vin.PubKey = make([]byte, lenVal)
		reader.Read(vin.PubKey)

		tx.Vin = append(tx.Vin, vin)
	}

	// Outputs
	var outputsCount int64
	binary.Read(reader, binary.BigEndian, &outputsCount)
	for i := 0; i < int(outputsCount); i++ {
		var vout TxOutput
		var lenVal int64

		binary.Read(reader, binary.BigEndian, &vout.Value)

		binary.Read(reader, binary.BigEndian, &lenVal)
		vout.PubKeyHash = make([]byte, lenVal)
		reader.Read(vout.PubKeyHash)

		tx.Vout = append(tx.Vout, vout)
	}

	// Timestamp
	if reader.Len() > 0 {
		binary.Read(reader, binary.BigEndian, &tx.Timestamp)
	}

	// Recalculate Hash (ID)
	tx.ID = tx.Hash()
	return tx
}

// Hash returns the hash of the Transaction
func (tx *Transaction) Hash() []byte {
	var hash [32]byte

	txCopy := *tx
	txCopy.ID = []byte{}

	hash = sha256.Sum256(txCopy.SerializeForHash())

	return hash[:]
}

// SerializeForHash returns a deterministic byte slice for hashing
func (tx Transaction) SerializeForHash() []byte {
	var encoded bytes.Buffer

	// Vin
	for _, vin := range tx.Vin {
		encoded.Write(vin.Txid)
		binary.Write(&encoded, binary.BigEndian, int64(vin.Vout))
		encoded.Write(vin.PubKey)
		encoded.Write(vin.Signature)
	}

	// Vout
	for _, vout := range tx.Vout {
		binary.Write(&encoded, binary.BigEndian, vout.Value)
		encoded.Write(vout.PubKeyHash)
	}

	// Timestamp
	binary.Write(&encoded, binary.BigEndian, tx.Timestamp)

	return encoded.Bytes()
}

// Sign signs each input of a Transaction
func (tx *Transaction) Sign(privKey ecdsa.PrivateKey, prevTXs map[string]Transaction) {
	if tx.IsCoinbase() {
		return
	}

	for _, vin := range tx.Vin {
		if prevTXs[hex.EncodeToString(vin.Txid)].ID == nil {
			log.Panic("ERROR: Previous transaction is not correct")
		}
	}

	txCopy := tx.TrimmedCopy()

	for inID, vin := range txCopy.Vin {
		prevTx := prevTXs[hex.EncodeToString(vin.Txid)]
		txCopy.Vin[inID].Signature = nil
		txCopy.Vin[inID].PubKey = prevTx.Vout[vin.Vout].PubKeyHash
		txCopy.ID = txCopy.Hash()
		txCopy.Vin[inID].PubKey = nil

		r, s, err := ecdsa.Sign(rand.Reader, &privKey, txCopy.ID)
		if err != nil {
			log.Panic(err)
		}
		rBytes := make([]byte, 32)
		sBytes := make([]byte, 32)
		r.FillBytes(rBytes)
		s.FillBytes(sBytes)
		signature := append(rBytes, sBytes...)

		tx.Vin[inID].Signature = signature
	}
}

// Verify verifies signatures of Transaction inputs
func (tx *Transaction) Verify(prevTXs map[string]Transaction) bool {
	if tx.IsCoinbase() {
		return true
	}

	for _, vin := range tx.Vin {
		if prevTXs[hex.EncodeToString(vin.Txid)].ID == nil {
			log.Panic("ERROR: Previous transaction is not correct")
		}
	}

	txCopy := tx.TrimmedCopy()
	curve := elliptic.P256()

	for inID, vin := range tx.Vin {
		prevTx := prevTXs[hex.EncodeToString(vin.Txid)]
		txCopy.Vin[inID].Signature = nil
		txCopy.Vin[inID].PubKey = prevTx.Vout[vin.Vout].PubKeyHash
		txCopy.ID = txCopy.Hash()
		txCopy.Vin[inID].PubKey = nil

		// Verify ownership: Check if the input signer's key hashes to the output's PubKeyHash
		signerHash := HashPubKey(vin.PubKey)
		if !bytes.Equal(signerHash, prevTx.Vout[vin.Vout].PubKeyHash) {
			return false
		}

		r := big.Int{}
		s := big.Int{}
		if len(vin.Signature) != 64 {
			return false
		}
		r.SetBytes(vin.Signature[:32])
		s.SetBytes(vin.Signature[32:])

		x := big.Int{}
		y := big.Int{}
		if len(vin.PubKey) != 64 {
			return false
		}
		x.SetBytes(vin.PubKey[:32])
		y.SetBytes(vin.PubKey[32:])

		rawPubKey := ecdsa.PublicKey{Curve: curve, X: &x, Y: &y}
		if !ecdsa.Verify(&rawPubKey, txCopy.ID, &r, &s) {
			return false
		}
	}

	return true
}

// TrimmedCopy creates a trimmed copy of Transaction to be used in signing
func (tx *Transaction) TrimmedCopy() Transaction {
	var inputs []TxInput
	var outputs []TxOutput

	for _, vin := range tx.Vin {
		inputs = append(inputs, TxInput{vin.Txid, vin.Vout, nil, nil})
	}

	for _, vout := range tx.Vout {
		outputs = append(outputs, TxOutput{vout.Value, vout.PubKeyHash})
	}

	txCopy := Transaction{tx.ID, inputs, outputs, tx.Timestamp}

	return txCopy
}

// IsCoinbase checks whether the transaction is coinbase
func (tx Transaction) IsCoinbase() bool {
	return len(tx.Vin) == 1 && len(tx.Vin[0].Txid) == 0 && tx.Vin[0].Vout == -1
}

// NewCoinbaseTX creates a new coinbase transaction
func NewCoinbaseTX(to, data string, amount int64) *Transaction {
	if data == "" {
		data = fmt.Sprintf("Reward to '%s'", to)
	}

	txin := TxInput{[]byte{}, -1, nil, []byte(data)}
	txout := NewTxOutput(amount, to)
	tx := Transaction{nil, []TxInput{txin}, []TxOutput{*txout}, time.Now().Unix()}
	tx.ID = tx.Hash()

	return &tx
}

// NewUTXOTransaction creates a new transaction
func NewUTXOTransaction(from, to string, amount int64, utxoSet *UTXOSet) *Transaction {
	var inputs []TxInput
	var outputs []TxOutput

	wallets, err := CreateWallets()
	if err != nil {
		log.Panic(err)
	}
	wallet := wallets.GetWalletRef(from)
	if wallet == nil {
		fmt.Printf("⛔ ERRORE: Wallet non trovato per l'indirizzo mittente %s. Assicurati di avere il file wallet.dat corretto.\n", from)
		os.Exit(1)
	}
	pubKeyHash := HashPubKey(wallet.PublicKey)

	acc, validOutputs := utxoSet.FindSpendableOutputs(pubKeyHash, amount)

	if acc < amount {
		fmt.Printf("⛔ ERRORE: Fondi insufficienti. Disponibili: %d, Richiesti: %d\n", acc, amount)
		os.Exit(1)
		// return nil // unreachable
	}

	for txid, outs := range validOutputs {
		txID, err := hex.DecodeString(txid)
		if err != nil {
			log.Panic(err)
		}

		for _, out := range outs {
			input := TxInput{txID, out, nil, wallet.PublicKey}
			inputs = append(inputs, input)
		}
	}

	outputs = append(outputs, *NewTxOutput(amount, to))

	if acc > amount {
		outputs = append(outputs, *NewTxOutput(acc-amount, from))
	}

	tx := Transaction{nil, inputs, outputs, time.Now().Unix()}
	tx.ID = tx.Hash()
	utxoSet.Blockchain.SignTransaction(&tx, wallet.GetPrivateKey())

	return &tx
}
