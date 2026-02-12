package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"log"
	"math/big"

	"golang.org/x/crypto/ripemd160"
)

const (
	version = byte(0x00) // Hex for '0', similar to Bitcoin
)

// Wallet stores private and public keys
type Wallet struct {
	PrivateKey []byte // x509 Marshaled
	PublicKey  []byte // Appended X and Y
}

// NewWallet creates and returns a Wallet
func NewWallet() *Wallet {
	private, public := newKeyPair()

	encodedPrivate, _ := x509.MarshalECPrivateKey(&private)

	wallet := Wallet{encodedPrivate, public}
	return &wallet
}

// MakeWalletFromPrivKeyHex creates a Wallet from a hex string private key
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

// GetAddress returns wallet address
func (w Wallet) GetAddress() []byte {
	pubKeyHash := HashPubKey(w.PublicKey)

	versionedPayload := append([]byte{version}, pubKeyHash...)
	checksum := checksum(versionedPayload)

	fullPayload := append(versionedPayload, checksum...)
	address := Base58Encode(fullPayload)
	return address
}

// GetPrivateKey returns the ECDSA Private Key
func (w Wallet) GetPrivateKey() ecdsa.PrivateKey {
	key, err := x509.ParseECPrivateKey(w.PrivateKey)
	if err != nil {
		log.Panic(err)
	}
	return *key
}

// HashPubKey hashes public key
func HashPubKey(pubKey []byte) []byte {
	publicSHA256 := sha256.Sum256(pubKey)

	RIPEMD160Hasher := ripemd160.New()
	_, err := RIPEMD160Hasher.Write(publicSHA256[:])
	if err != nil {
		log.Panic(err)
	}
	publicRIPEMD160 := RIPEMD160Hasher.Sum(nil)

	return publicRIPEMD160
}

func newKeyPair() (ecdsa.PrivateKey, []byte) {
	curve := elliptic.P256()
	private, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		log.Panic(err)
	}
	// Use elliptic.Marshal for consistency
	pubKey := elliptic.Marshal(curve, private.PublicKey.X, private.PublicKey.Y)

	return *private, pubKey
}

// ValidateAddress validates if address is valid
func ValidateAddress(address string) bool {
	pubKeyHash, err := Base58Decode([]byte(address))
	if err != nil {
		return false
	}
	if len(pubKeyHash) < 4 {
		return false
	}
	actualChecksum := pubKeyHash[len(pubKeyHash)-4:]
	version := pubKeyHash[0]
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4]
	targetChecksum := checksum(append([]byte{version}, pubKeyHash...))

	return bytes.Equal(actualChecksum, targetChecksum)
}
