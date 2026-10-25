package core

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"math/big"
	"time"
)

const (
	MaxTxSize             = 100 * 1024          // 100 KB max transaction payload size
	MaxFieldLen           = MaxTxSize           // Align field length limit to max transaction size
	MinTxInputSize        = 32                  // Minimum serialized input size (4 int64 length/value prefixes)
	MinTxOutputSize       = 16                  // Minimum serialized output size (2 int64 length/value prefixes)
	MaxTxInputs           = 2000                // Maximum inputs per transaction
	MaxTxOutputs          = 2000                // Maximum outputs per transaction
	MaxMoney        int64 = 8910000 * 100000000 // Maximum supply in atomic units
)

type TxOutput struct {
	Value      int64
	PubKeyHash []byte
}

func (out *TxOutput) Lock(address []byte) error {
	pubKeyHash, err := ExtractPubKeyHash(string(address))
	if err != nil {
		return err
	}
	out.PubKeyHash = pubKeyHash
	return nil
}

func (out *TxOutput) IsLockedWithKey(pubKeyHash []byte) bool {
	return bytes.Equal(out.PubKeyHash, pubKeyHash)
}

func (out *TxOutput) IsOPReturn() bool {
	return out.Value == 0
}

func NewTxOutput(value int64, address string) (*TxOutput, error) {
	if value < 0 || value > MaxMoney {
		return nil, fmt.Errorf("output value %d is outside the valid range [0, %d]", value, MaxMoney)
	}

	txo := &TxOutput{value, nil}
	err := txo.Lock([]byte(address))
	if err != nil {
		return nil, err
	}
	return txo, nil
}

type TxInput struct {
	Txid      []byte
	Vout      int
	Signature []byte
	PubKey    []byte
}

type Transaction struct {
	ID        []byte
	Vin       []TxInput
	Vout      []TxOutput
	Timestamp int64
}

// ValidateAmounts checks that every output and the transaction output total
// are within the consensus monetary limit.
func (tx Transaction) ValidateAmounts() error {
	var total int64
	for i, output := range tx.Vout {
		if output.Value < 0 {
			return fmt.Errorf("output %d has negative value %d", i, output.Value)
		}
		if output.Value > MaxMoney-total {
			return fmt.Errorf("transaction output total exceeds maximum money supply %d", MaxMoney)
		}
		total += output.Value
	}
	return nil
}

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

func DeserializeTransaction(data []byte) (Transaction, error) {
	var tx Transaction

	if len(data) > MaxTxSize {
		return tx, fmt.Errorf("transaction size %d exceeds maximum allowed %d", len(data), MaxTxSize)
	}

	reader := bytes.NewReader(data)

	readLen := func(field string) (int64, error) {
		var v int64
		if err := binary.Read(reader, binary.BigEndian, &v); err != nil {
			return 0, fmt.Errorf("read %s length: %w", field, err)
		}
		if v < 0 || v > MaxFieldLen {
			return 0, fmt.Errorf("invalid %s length %d", field, v)
		}
		if v > int64(reader.Len()) {
			return 0, fmt.Errorf("truncated %s: need %d bytes, have %d", field, v, reader.Len())
		}
		return v, nil
	}

	// Inputs
	var inputsCount int64
	if err := binary.Read(reader, binary.BigEndian, &inputsCount); err != nil {
		return tx, fmt.Errorf("read input count: %w", err)
	}
	if inputsCount < 0 || inputsCount > MaxTxInputs {
		return tx, fmt.Errorf("invalid input count %d", inputsCount)
	}

	if inputsCount > 0 {
		minBytesRequired := inputsCount * MinTxInputSize
		if int64(reader.Len()) < minBytesRequired {
			return tx, fmt.Errorf("insufficient data for %d inputs: need at least %d bytes, have %d", inputsCount, minBytesRequired, reader.Len())
		}
		tx.Vin = make([]TxInput, 0, inputsCount)
	}

	for i := 0; i < int(inputsCount); i++ {
		var vin TxInput

		// TxID
		l, err := readLen("transaction ID")
		if err != nil {
			return tx, err
		}
		vin.Txid = make([]byte, int(l))
		if _, err := io.ReadFull(reader, vin.Txid); err != nil {
			return tx, fmt.Errorf("read transaction ID: %w", err)
		}

		// Vout
		var vout int64
		if err := binary.Read(reader, binary.BigEndian, &vout); err != nil {
			return tx, fmt.Errorf("read output index: %w", err)
		}
		if vout < 0 {
			if !(inputsCount == 1 && len(vin.Txid) == 0 && vout == -1) {
				return tx, fmt.Errorf("invalid output index %d: cannot be negative", vout)
			}
		} else if vout > math.MaxInt32 || int64(int(vout)) != vout {
			return tx, fmt.Errorf("output index %d overflows int", vout)
		} else if vout >= MaxTxOutputs {
			return tx, fmt.Errorf("output index %d exceeds maximum allowed outputs %d", vout, MaxTxOutputs)
		}
		vin.Vout = int(vout)

		// Signature
		l, err = readLen("signature")
		if err != nil {
			return tx, err
		}
		vin.Signature = make([]byte, int(l))
		if _, err := io.ReadFull(reader, vin.Signature); err != nil {
			return tx, fmt.Errorf("read signature: %w", err)
		}

		// PubKey
		l, err = readLen("public key")
		if err != nil {
			return tx, err
		}
		vin.PubKey = make([]byte, int(l))
		if _, err := io.ReadFull(reader, vin.PubKey); err != nil {
			return tx, fmt.Errorf("read public key: %w", err)
		}

		tx.Vin = append(tx.Vin, vin)
	}

	// Outputs
	var outputsCount int64
	if err := binary.Read(reader, binary.BigEndian, &outputsCount); err != nil {
		return tx, fmt.Errorf("read output count: %w", err)
	}
	if outputsCount < 0 || outputsCount > MaxTxOutputs {
		return tx, fmt.Errorf("invalid output count %d", outputsCount)
	}

	if outputsCount > 0 {
		minBytesRequired := outputsCount * MinTxOutputSize
		if int64(reader.Len()) < minBytesRequired {
			return tx, fmt.Errorf("insufficient data for %d outputs: need at least %d bytes, have %d", outputsCount, minBytesRequired, reader.Len())
		}
		tx.Vout = make([]TxOutput, 0, outputsCount)
	}

	for i := 0; i < int(outputsCount); i++ {
		var vout TxOutput

		if err := binary.Read(reader, binary.BigEndian, &vout.Value); err != nil {
			return tx, fmt.Errorf("read output value: %w", err)
		}

		l, err := readLen("public key hash")
		if err != nil {
			return tx, err
		}
		vout.PubKeyHash = make([]byte, int(l))
		if _, err := io.ReadFull(reader, vout.PubKeyHash); err != nil {
			return tx, fmt.Errorf("read public key hash: %w", err)
		}

		tx.Vout = append(tx.Vout, vout)
	}

	if err := binary.Read(reader, binary.BigEndian, &tx.Timestamp); err != nil {
		return tx, fmt.Errorf("read timestamp: %w", err)
	}
	if reader.Len() != 0 {
		return tx, fmt.Errorf("transaction has %d trailing bytes", reader.Len())
	}
	if err := tx.ValidateAmounts(); err != nil {
		return tx, fmt.Errorf("invalid transaction amounts: %w", err)
	}

	// Recalculate Hash (ID)
	tx.ID = tx.Hash()
	return tx, nil
}

func (tx *Transaction) Hash() []byte {
	var hash [32]byte

	txCopy := *tx
	txCopy.ID = []byte{}

	hash = sha256.Sum256(txCopy.SerializeForHash())

	return hash[:]
}

func (tx Transaction) SerializeForHash() []byte {
	var encoded bytes.Buffer
	var integer [8]byte
	writeInt64 := func(value int64) {
		binary.BigEndian.PutUint64(integer[:], uint64(value))
		encoded.Write(integer[:])
	}
	writeBytes := func(value []byte) {
		writeInt64(int64(len(value)))
		encoded.Write(value)
	}

	// Inputs
	writeInt64(int64(len(tx.Vin)))
	for _, vin := range tx.Vin {
		writeBytes(vin.Txid)
		writeInt64(int64(vin.Vout))
		writeBytes(vin.Signature)
		writeBytes(vin.PubKey)
	}

	// Outputs
	writeInt64(int64(len(tx.Vout)))
	for _, vout := range tx.Vout {
		writeInt64(vout.Value)
		writeBytes(vout.PubKeyHash)
	}

	writeInt64(tx.Timestamp)

	return encoded.Bytes()
}

func (tx *Transaction) Sign(privKey ecdsa.PrivateKey, prevTXs map[string]Transaction) error {
	if err := tx.ValidateAmounts(); err != nil {
		return fmt.Errorf("invalid transaction amounts: %w", err)
	}
	if tx.IsCoinbase() {
		return nil
	}

	for inID, vin := range tx.Vin {
		prevTx, exists := prevTXs[hex.EncodeToString(vin.Txid)]
		if !exists || prevTx.ID == nil {
			return fmt.Errorf("skipped input: previous transaction %x not found in context", vin.Txid)
		}
		if vin.Vout < 0 || vin.Vout >= len(prevTx.Vout) {
			return fmt.Errorf("input %d references invalid output index %d (previous transaction has %d outputs)", inID, vin.Vout, len(prevTx.Vout))
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
			return fmt.Errorf("ECDSA signing failed: %w", err)
		}

		curveOrder := privKey.Curve.Params().N
		halfOrder := new(big.Int).Rsh(curveOrder, 1)
		if s.Cmp(halfOrder) > 0 {
			s.Sub(curveOrder, s)
		}

		rBytes := make([]byte, 32)
		sBytes := make([]byte, 32)
		r.FillBytes(rBytes)
		s.FillBytes(sBytes)
		signature := append(rBytes, sBytes...)

		tx.Vin[inID].Signature = signature
	}

	tx.ID = tx.Hash()
	return nil
}

func (tx *Transaction) Verify(prevTXs map[string]Transaction) bool {
	if err := tx.ValidateAmounts(); err != nil {
		fmt.Printf("⛔ ERROR: Invalid transaction amounts: %v\n", err)
		return false
	}
	if tx.IsCoinbase() {
		return true
	}

	if len(tx.ID) > 0 && !bytes.Equal(tx.ID, tx.Hash()) {
		fmt.Printf("⛔ ERROR: Transaction ID mismatch (got %x, expected %x)\n", tx.ID, tx.Hash())
		return false
	}

	for inID, vin := range tx.Vin {
		prevTx, exists := prevTXs[hex.EncodeToString(vin.Txid)]
		if !exists || prevTx.ID == nil {
			fmt.Printf("ERROR: Previous transaction is not correct\n")
			return false
		}
		if vin.Vout < 0 || vin.Vout >= len(prevTx.Vout) {
			fmt.Printf("⛔ ERROR: Input %d references invalid output index %d (previous transaction has %d outputs)\n", inID, vin.Vout, len(prevTx.Vout))
			return false
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

		// 1. Strict Key Check: ANSI X9.62 Uncompressed (65 bytes, 0x04 prefix)
		if len(vin.PubKey) != 65 {
			fmt.Printf("⛔ ERROR: Input %d: Invalid Public Key length: %d (Expected 65)\n", inID, len(vin.PubKey))
			return false
		}
		if vin.PubKey[0] != 0x04 {
			fmt.Printf("⛔ ERROR: Input %d: Invalid Public Key prefix: 0x%x (Expected 0x04)\n", inID, vin.PubKey[0])
			return false
		}

		// Verify ownership: Check if the input signer's key hashes to the output's PubKeyHash
		signerHash := HashPubKey(vin.PubKey)
		if !bytes.Equal(signerHash, prevTx.Vout[vin.Vout].PubKeyHash) {
			fmt.Printf("⛔ ERROR: Input %d: Public Key hash does not match Output's PubKeyHash\n", inID)
			return false
		}

		r := big.Int{}
		s := big.Int{}
		if len(vin.Signature) != 64 {
			fmt.Printf("⛔ ERROR: Input %d: Invalid Signature length: %d\n", inID, len(vin.Signature))
			return false
		}
		r.SetBytes(vin.Signature[:32])
		s.SetBytes(vin.Signature[32:])

		curveOrder := curve.Params().N
		halfOrder := new(big.Int).Rsh(curveOrder, 1)
		if s.Cmp(halfOrder) > 0 {
			fmt.Printf("⛔ ERROR: Input %d: Non-canonical signature (high S malleability)\n", inID)
			return false
		}

		x := big.Int{}
		y := big.Int{}
		// We already checked len(vin.PubKey) == 65 above.
		// Uncompressed format: 0x04 + 32 bytes X + 32 bytes Y
		x.SetBytes(vin.PubKey[1:33])
		y.SetBytes(vin.PubKey[33:])

		rawPubKey := ecdsa.PublicKey{Curve: curve, X: &x, Y: &y}
		if !ecdsa.Verify(&rawPubKey, txCopy.ID, &r, &s) {
			fmt.Printf("⛔ ERROR: Input %d: ECDSA Signature Verification failed\n", inID)
			return false
		}
	}

	return true
}

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

func (tx Transaction) IsCoinbase() bool {
	return len(tx.Vin) == 1 && len(tx.Vin[0].Txid) == 0 && tx.Vin[0].Vout == -1
}

func NewCoinbaseTX(to, data string, amount int64) (*Transaction, error) {
	if data == "" {
		data = fmt.Sprintf("Reward to '%s'", to)
	}

	txin := TxInput{[]byte{}, -1, nil, []byte(data)}
	txout, err := NewTxOutput(amount, to)
	if err != nil {
		return nil, err
	}
	tx := Transaction{nil, []TxInput{txin}, []TxOutput{*txout}, time.Now().Unix()}
	tx.ID = tx.Hash()

	return &tx, nil
}

type MempoolItem struct {
	Tx       Transaction
	AddedAt  int64
	PeerID   string
	ByteSize int
}

// Size returns the byte size of the mempool item.
// If ByteSize is explicitly recorded, it is returned; otherwise it falls back
// to the serialized length of the transaction.
func (item MempoolItem) Size() int {
	if item.ByteSize > 0 {
		return item.ByteSize
	}
	return len(item.Tx.Serialize())
}
