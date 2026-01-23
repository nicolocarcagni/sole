package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
)

// AuthorizedValidators contains the hex-encoded public keys of authorized validators
// Each entry is 128 hex characters (64 bytes = 32 bytes X + 32 bytes Y)
var AuthorizedValidators = []string{
	"e5312b9e0d2dd90695cf2fc536995adf41881f465c90ad1e674dac65af8996140e88c903ebf228704f54b9512f9ed0a72d79c99d5af68dda8f4a897ea77888ab",
	"a1c546661719fc83cc614f776ef2ae65b5e2f06fb351de9f077507da3cf2c4bfae617340120591e6f670676de2a42d741883dc67355a4ef1ba5c8a9029a8b33f",
	"f5259938c7d873b109a458065a4b442c28abca3667c173b73d38219d58163d988cac1464fb43fbc4d49d430fb1e3f6da33642b28288f019d7e9e6ff5e8a16a46", // Auto-added validator
	// Example: "deadbeef..."
}

// IsAuthorizedValidator checks if the given public key is in the authorized list
func IsAuthorizedValidator(pubKeyHex string) bool {
	for _, v := range AuthorizedValidators {
		if v == pubKeyHex {
			return true
		}
	}
	return false
}

// SignBlock signs the block hash with the validator's private key
func SignBlock(block *Block, privKey ecdsa.PrivateKey) error {
	// Ensure hash is set
	if len(block.Hash) == 0 {
		block.SetHash()
	}

	r, s, err := ecdsa.Sign(rand.Reader, &privKey, block.Hash)
	if err != nil {
		return err
	}

	// Store signature as 64 bytes (32 for R + 32 for S)
	rBytes := make([]byte, 32)
	sBytes := make([]byte, 32)
	r.FillBytes(rBytes)
	s.FillBytes(sBytes)

	block.Signature = append(rBytes, sBytes...)
	block.Validator = append(privKey.PublicKey.X.FillBytes(make([]byte, 32)),
		privKey.PublicKey.Y.FillBytes(make([]byte, 32))...)

	return nil
}

// VerifyBlockSignature verifies that the block signature is valid
func VerifyBlockSignature(block *Block) bool {
	if len(block.Signature) != 64 {
		fmt.Println("PoA: Invalid signature length")
		return false
	}
	if len(block.Validator) != 64 {
		fmt.Println("PoA: Invalid validator length")
		return false
	}

	// Check if validator is authorized
	validatorHex := hex.EncodeToString(block.Validator)
	if !IsAuthorizedValidator(validatorHex) {
		fmt.Printf("PoA: Validator %s is not authorized\n", validatorHex[:16]+"...")
		return false
	}

	// Reconstruct public key from Validator bytes
	curve := elliptic.P256()
	x := new(big.Int).SetBytes(block.Validator[:32])
	y := new(big.Int).SetBytes(block.Validator[32:])
	pubKey := ecdsa.PublicKey{Curve: curve, X: x, Y: y}

	// Extract R and S from signature
	r := new(big.Int).SetBytes(block.Signature[:32])
	s := new(big.Int).SetBytes(block.Signature[32:])

	// Recompute block hash (exclude Signature field)
	tempBlock := *block
	tempBlock.Signature = nil
	tempBlock.SetHash()

	// Verify
	if !ecdsa.Verify(&pubKey, tempBlock.Hash, r, s) {
		fmt.Println("PoA: Block signature verification failed")
		return false
	}

	return true
}

// GetValidatorHex returns the hex-encoded public key for a wallet
func GetValidatorHex(w Wallet) string {
	return hex.EncodeToString(w.PublicKey)
}
