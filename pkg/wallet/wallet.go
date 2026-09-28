package wallet

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/tyler-smith/go-bip39"

	"github.com/nicolocarcagni/sole/pkg/core"
)

const ()

type Wallet struct {
	PrivateKey []byte // x509 Marshaled
	PublicKey  []byte // Appended X and Y
}

func NewMnemonic() (string, error) {
	entropy, err := bip39.NewEntropy(128)
	if err != nil {
		return "", err
	}
	return bip39.NewMnemonic(entropy)
}

func MakeWalletFromMnemonic(mnemonic string) (*Wallet, error) {
	mnemonic = strings.TrimSpace(mnemonic)

	if len(strings.Fields(mnemonic)) != 12 {
		return nil, errors.New("invalid mnemonic: must be exactly 12 words")
	}

	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, errors.New("invalid mnemonic: checksum failed or words are not in the BIP39 dictionary")
	}

	seed := bip39.NewSeed(mnemonic, "")
	privKeyBytes := sha256.Sum256(seed)

	curve := elliptic.P256()
	privKey := new(ecdsa.PrivateKey)
	privKey.D = new(big.Int).SetBytes(privKeyBytes[:])
	privKey.PublicKey.Curve = curve
	privKey.PublicKey.X, privKey.PublicKey.Y = curve.ScalarBaseMult(privKeyBytes[:])

	encodedPrivate, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return nil, err
	}

	pubKey := elliptic.Marshal(curve, privKey.PublicKey.X, privKey.PublicKey.Y)

	return &Wallet{encodedPrivate, pubKey}, nil
}

func NewWallet() (*Wallet, string, error) {
	mnemonic, err := NewMnemonic()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate mnemonic: %w", err)
	}

	wallet, err := MakeWalletFromMnemonic(mnemonic)
	if err != nil {
		return nil, "", fmt.Errorf("failed to make wallet from mnemonic: %w", err)
	}

	return wallet, mnemonic, nil
}

func MakeWalletFromPrivKeyHex(privKeyHex string) (*Wallet, error) {
	// 1. Decode Hex
	privKeyBytes, err := hex.DecodeString(privKeyHex)
	if err != nil {
		return nil, err
	}

	// 2. Reconstruct ecdsa.PrivateKey
	curve := elliptic.P256()
	privKey := new(ecdsa.PrivateKey)
	privKey.D = new(big.Int).SetBytes(privKeyBytes)
	privKey.PublicKey.Curve = curve
	privKey.PublicKey.X, privKey.PublicKey.Y = curve.ScalarBaseMult(privKeyBytes)

	// 3. Encode Private Key for storage (x509)
	encodedPrivate, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return nil, err
	}

	// 4. Construct Public Key Bytes (for Address generation)
	// Use elliptic.Marshal to get the uncompressed format (0x04 prefix)
	// This matches the behavior of vanity.go and standard tools
	pubKey := elliptic.Marshal(curve, privKey.PublicKey.X, privKey.PublicKey.Y)

	// 5. Return Wallet
	wallet := Wallet{encodedPrivate, pubKey}
	return &wallet, nil
}

func (w Wallet) GetAddress() string {
	pubKeyHash := core.HashPubKey(w.PublicKey)
	return core.AddressFromPubKeyHash(pubKeyHash)
}

func (w Wallet) GetPrivateKey() (ecdsa.PrivateKey, error) {
	key, err := x509.ParseECPrivateKey(w.PrivateKey)
	if err != nil {
		return ecdsa.PrivateKey{}, err
	}
	return *key, nil
}

func ValidateAddress(address string) bool {
	pubKeyHash, err := core.Base58Decode([]byte(address))
	if err != nil {
		return false
	}
	if len(pubKeyHash) < 4 {
		return false
	}
	actualChecksum := pubKeyHash[len(pubKeyHash)-4:]
	version := pubKeyHash[0]
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4]
	targetChecksum := core.Checksum(append([]byte{version}, pubKeyHash...))

	return bytes.Equal(actualChecksum, targetChecksum)
}

func (w *Wallet) GetValidatorHex() string {
	return hex.EncodeToString(w.PublicKey)
}
