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
	"033cc6d258184de4a0dfd8a29419074c6dccfe97753344467e3713a8c27c9aa260e9017e76ae34208eb768237ddec480d80f0f7e81c53ce27ccb3073eee37a84", // Rettore
	"5b28a2d412b9fa6dba48f89c42e63478c81d8539a3e65293418f8ec8ef09b6f5d615b4432ff4b3e0cfa9e110d1c1ad21f4a5592bbd6b3701721647eccdf2b3a9", // Mensa
	"31b27f9828e1ae5ec364277bf690bdfb8eaa184d289c336cd7471dfb0e9b8d6de640e54fa5ff649597dfe9f79502a92fccc21e071d627f1587f8f0e5d872e4f0", // Ingegneria
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
