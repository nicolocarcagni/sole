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
// Each entry is 130 hex characters (65 bytes = 1 byte Prefix [0x04] + 32 bytes X + 32 bytes Y)
var AuthorizedValidators = []string{
	"0499962080b1c07db1ecb7f2d58978203dfe5eede8e648c3755afed392fec7716d8c7a0fe455d15d64b8dd1363d60c78926e9dce4aad2e08a0006cd50215cb87c3", // Foundation
	"046b936a4fc7f0ed3d37eaeb5f95b7cac901c6a3b6c4bbd377fbefa525812a8cc2918d738d3ba24ba5b5368ed6a91f23bda663c9763f8969880df5c9af5451bf4d",
	"04d6e939245ddd571c20a585020507ec829384a02a27b0d3f3279d44a21d855c49f58644e95ada1046f14999e0e6be831d25b58eae7bfcfba3ba01643a5b771879",
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

// GetSignatureBytes returns (r, s) as 64 bytes with zero padding
func GetSignatureBytes(r, s *big.Int) []byte {
	rBytes := r.Bytes()
	sBytes := s.Bytes()

	sigBytes := make([]byte, 64)
	// Copy r to first 32 bytes (right aligned if needed, but big.Int.Bytes is purely value.
	// We need 32 bytes. If len > 32 (rare with P256 but possible if leading bit is 1 and interpreted as signed? No, ECDSA is unsigned)
	// If len < 32, we pad with leading zeros.
	// copy() matches indices.
	// To pad left:
	// copy(sigBytes[32-len(rBytes):32], rBytes)
	copy(sigBytes[32-len(rBytes):32], rBytes)
	copy(sigBytes[64-len(sBytes):64], sBytes)

	return sigBytes
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

	block.Signature = GetSignatureBytes(r, s)
	block.Validator = append(privKey.PublicKey.X.FillBytes(make([]byte, 32)),
		privKey.PublicKey.Y.FillBytes(make([]byte, 32))...)

	return nil
}

// VerifyBlockSignature verifies that the block signature is valid
func VerifyBlockSignature(block *Block) bool {
	if len(block.Signature) != 64 {
		fmt.Printf("PoA: Invalid signature length. Expected 64, Got %d\n", len(block.Signature))
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

	// Extract R and S from signature (fixed 32 bytes each)
	r := new(big.Int).SetBytes(block.Signature[:32])
	s := new(big.Int).SetBytes(block.Signature[32:])

	// Verify STRICTLY against the Block Hash (as signed by Validator)
	// We trust the Hash integrity is checked elsewhere or we accept the Hash as the identity.
	if !ecdsa.Verify(&pubKey, block.Hash, r, s) {
		fmt.Printf("PoA: Block signature verification failed. len(sig)=%d\n", len(block.Signature))
		return false
	}

	return true
}

// GetValidatorHex returns the hex-encoded public key for a wallet
func GetValidatorHex(w Wallet) string {
	return hex.EncodeToString(w.PublicKey)
}
