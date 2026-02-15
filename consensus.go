package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"
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

	// PATCH: Handle both Raw (64 bytes) and Standard (65 bytes) Public Keys
	var pubKeyBytes []byte
	var x, y *big.Int

	if len(block.Validator) == 64 {
		// Log detection of old format (optional but helpful)
		// fmt.Println("PoA: Detected Raw Public Key (64 bytes). Normalizing...")

		// Normalize to Standard Format (Prefix 0x04)
		pubKeyBytes = append([]byte{0x04}, block.Validator...)
		x = new(big.Int).SetBytes(block.Validator[:32])
		y = new(big.Int).SetBytes(block.Validator[32:])
	} else if len(block.Validator) == 65 {
		if block.Validator[0] != 0x04 {
			fmt.Printf("PoA: Invalid Standard Key Prefix. Expected 0x04, Got 0x%x\n", block.Validator[0])
			return false
		}
		pubKeyBytes = block.Validator
		x = new(big.Int).SetBytes(block.Validator[1:33])
		y = new(big.Int).SetBytes(block.Validator[33:])
	} else {
		fmt.Printf("PoA: Invalid validator length. Expected 64 or 65, Got %d\n", len(block.Validator))
		return false
	}

	// Check if validator is authorized using the NORMALIZED (Standard) Hex string
	validatorHex := hex.EncodeToString(pubKeyBytes)
	if !IsAuthorizedValidator(validatorHex) {
		fmt.Printf("PoA: Validator %s... is not authorized\n", validatorHex[:16])
		return false
	}

	// Reconstruct public key from Validator bytes
	curve := elliptic.P256()
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

// --- PoA Hardening: Temporal Validation & Anti-Spam ---

const (
	// DriftTolerance is the allowed time difference for block timestamp
	DriftTolerance = 1 * time.Minute
	// PoADifficulty is the number of leading zero bits required (symbolic PoW)
	// For educational efficiency, we use a simple check (e.g., Hash starts with 0x0...)
	// Here we check if the first N bytes are 0. Let's say 1 byte (buffer[0] == 0) for very easy,
	// or 2 bytes for harder.
	// User requested "Starts with at least 1 zero or 4 bit a zero".
	// Let's require the first 2 hex chars (1 byte) to be 00.
	TargetZeros = 1 // Leading bytes must be 0x00
)

// MineBlock performs the "Mining" (finding a valid Nonce)
func MineBlock(block *Block) {
	fmt.Printf("⛏️  Mining block %d... ", block.Height)
	block.Nonce = 0

	for {
		block.SetHash()
		// Check difficulty
		if CheckProofOfWork(block.Hash) {
			break
		}
		block.Nonce++
	}
	fmt.Printf("Done! Nonce: %d\n", block.Nonce)
}

// CheckProofOfWork checks if the hash satisfies the difficulty
func CheckProofOfWork(hash []byte) bool {
	// Simple check: First byte must be 0
	if len(hash) < TargetZeros {
		return false
	}
	for i := 0; i < TargetZeros; i++ {
		if hash[i] != 0x00 {
			return false
		}
	}
	return true
}

// ValidateBlockHeader checks strict PoA rules (Timestamp, Drift, Proof)
func ValidateBlockHeader(block *Block, prevBlock *Block) error {
	// 1. Monotonic Timestamp
	if block.Timestamp <= prevBlock.Timestamp {
		return fmt.Errorf("timestamp is not monotonic (Current: %d, Prev: %d)", block.Timestamp, prevBlock.Timestamp)
	}

	// 2. Drift Tolerance (Future Check)
	now := time.Now().Unix()
	if block.Timestamp > now+int64(DriftTolerance.Seconds()) {
		return fmt.Errorf("timestamp too far in future (Block: %d, Now: %d, Limit: %d)", block.Timestamp, now, int64(DriftTolerance.Seconds()))
	}

	// 3. Anti-Spam (Proof of Work)
	if !CheckProofOfWork(block.Hash) {
		return fmt.Errorf("invalid PoA Proof-of-Work (Hash: %x)", block.Hash)
	}

	return nil
}
