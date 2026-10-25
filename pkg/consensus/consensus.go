package consensus

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"sort"
	"time"

	"github.com/nicolocarcagni/sole/pkg/core"
)

// AuthorizedValidators contains the hex-encoded public keys of authorized validators
// Each entry is 130 hex characters (65 bytes = 1 byte Prefix [0x04] + 32 bytes X + 32 bytes Y)
var AuthorizedValidators = []string{
	"0499962080b1c07db1ecb7f2d58978203dfe5eede8e648c3755afed392fec7716d8c7a0fe455d15d64b8dd1363d60c78926e9dce4aad2e08a0006cd50215cb87c3", // Foundation
	"046b936a4fc7f0ed3d37eaeb5f95b7cac901c6a3b6c4bbd377fbefa525812a8cc2918d738d3ba24ba5b5368ed6a91f23bda663c9763f8969880df5c9af5451bf4d",
	"04d6e939245ddd571c20a585020507ec829384a02a27b0d3f3279d44a21d855c49f58644e95ada1046f14999e0e6be831d25b58eae7bfcfba3ba01643a5b771879",
	// Example: "deadbeef..."
}

func IsAuthorizedValidator(pubKeyHex string) bool {
	for _, v := range AuthorizedValidators {
		if v == pubKeyHex {
			return true
		}
	}
	return false
}

func GetSignatureBytes(r, s *big.Int) []byte {
	rBytes := r.Bytes()
	sBytes := s.Bytes()

	sigBytes := make([]byte, 64)
	copy(sigBytes[32-len(rBytes):32], rBytes)
	copy(sigBytes[64-len(sBytes):64], sBytes)

	return sigBytes
}

func SignBlock(block *core.Block, privKey ecdsa.PrivateKey) error {
	// Derive the 64-byte raw public key (X‖Y, 32 bytes each) from the private key.
	keyX := privKey.PublicKey.X.FillBytes(make([]byte, 32))
	keyY := privKey.PublicKey.Y.FillBytes(make([]byte, 32))
	derivedPubKey := append(keyX, keyY...)

	if len(block.Validator) == 0 {
		// Validator not yet set: bind the public key into the header and recompute hash.
		block.Validator = derivedPubKey
		block.SetHash()
	} else {
		// Validator already set (e.g. by ForgeBlock): verify it matches the signing key.
		if !bytes.Equal(block.Validator, derivedPubKey) {
			return fmt.Errorf("SignBlock: key mismatch — block.Validator does not match privKey.PublicKey")
		}
		// Ensure the stored hash actually commits to the current header fields.
		if len(block.Hash) == 0 || !bytes.Equal(block.Hash, block.CalculateHash()) {
			block.SetHash()
		}
	}

	r, s, err := ecdsa.Sign(rand.Reader, &privKey, block.Hash)
	if err != nil {
		return err
	}

	block.Signature = GetSignatureBytes(r, s)
	return nil
}

func VerifyBlockSignature(block *core.Block) bool {
	if len(block.Signature) != 64 {
		fmt.Printf("PoA: Invalid signature length. Expected 64, Got %d\n", len(block.Signature))
		return false
	}

	// Handle both Raw (64 bytes) and Standard (65 bytes) Public Keys seamlessly
	var pubKeyBytes []byte
	var x, y *big.Int

	if len(block.Validator) == 64 {
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

	validatorHex := hex.EncodeToString(pubKeyBytes)
	if !IsAuthorizedValidator(validatorHex) {
		fmt.Printf("PoA: Validator %s... is not authorized\n", validatorHex[:16])
		return false
	}

	curve := elliptic.P256()
	pubKey := ecdsa.PublicKey{Curve: curve, X: x, Y: y}

	r := new(big.Int).SetBytes(block.Signature[:32])
	s := new(big.Int).SetBytes(block.Signature[32:])

	if !ecdsa.Verify(&pubKey, block.Hash, r, s) {
		fmt.Printf("PoA: core.Block signature verification failed. len(sig)=%d\n", len(block.Signature))
		return false
	}

	return true
}

// --- PoA Hardening: Temporal Validation & Anti-Spam ---

const (
	// DriftTolerance is the maximum allowed timestamp advance from a block's parent.
	DriftTolerance = 1 * time.Minute
	// MedianTimeWindow is the maximum number of recent block timestamps used
	// to establish the lower bound for a candidate block.
	MedianTimeWindow = 11
	// DefaultDifficulty is the number of leading zero bytes required for new blocks.
	DefaultDifficulty uint8 = core.DefaultDifficulty
	// TargetZeros is retained for callers that use the original default PoW target.
	TargetZeros = 1
	// MaxDifficulty bounds consensus work calculation and prevents impossible targets.
	MaxDifficulty uint8 = 8
)

func MineBlock(block *core.Block) {
	fmt.Printf("⛏️  Mining block %d... ", block.Height)
	if block.Difficulty == 0 {
		block.Difficulty = DefaultDifficulty
	}
	block.Nonce = 0

	for {
		block.SetHash()
		// Check difficulty
		if CheckProofOfWork(block.Hash, block.Difficulty) {
			break
		}
		block.Nonce++
	}
	fmt.Printf("Done! Nonce: %d\n", block.Nonce)
}

func CheckProofOfWork(hash []byte, difficulty ...uint8) bool {
	target := DefaultDifficulty
	if len(difficulty) > 0 {
		target = difficulty[0]
	}
	if target == 0 || target > MaxDifficulty || len(hash) < int(target) {
		return false
	}
	for i := 0; i < int(target); i++ {
		if hash[i] != 0x00 {
			return false
		}
	}
	return true
}

// BlockWork returns the expected work represented by a valid block difficulty.
func BlockWork(difficulty uint8) (*big.Int, error) {
	if difficulty == 0 || difficulty > MaxDifficulty {
		return nil, fmt.Errorf("difficulty %d is outside the supported range", difficulty)
	}
	return new(big.Int).Lsh(big.NewInt(1), uint(difficulty)*8), nil
}

func ValidateBlockHeader(block *core.Block, prevBlock *core.Block, ancestors ...*core.Block) error {
	if block == nil || prevBlock == nil {
		return fmt.Errorf("block and parent are required for header validation")
	}
	if block.Difficulty == 0 || block.Difficulty > MaxDifficulty {
		return fmt.Errorf("invalid block difficulty: %d", block.Difficulty)
	}
	// 0. Strict Hash Verification — the stored hash must equal the recomputed hash.
	// This is the primary defence against header malleability: any tampered field
	// (Validator, transactions, Nonce, Timestamp, Height, PrevBlockHash) will
	// produce a different CalculateHash() value and be rejected here.
	expectedHash := block.CalculateHash()
	if !bytes.Equal(block.Hash, expectedHash) {
		return fmt.Errorf("block hash mismatch: stored hash %x does not match calculated hash %x", block.Hash, expectedHash)
	}

	// 1. Median Time Past: candidate timestamps must advance beyond the median
	// of the parent and up to ten preceding ancestors.
	timestamps := make([]int64, 0, MedianTimeWindow)
	timestamps = append(timestamps, prevBlock.Timestamp)
	expectedHeight := prevBlock.Height - 1
	for _, ancestor := range ancestors {
		if len(timestamps) == MedianTimeWindow {
			break
		}
		if ancestor == nil || ancestor.Height != expectedHeight {
			return fmt.Errorf("invalid timestamp history: expected ancestor at height %d", expectedHeight)
		}
		timestamps = append(timestamps, ancestor.Timestamp)
		expectedHeight--
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })
	medianTimestamp := timestamps[len(timestamps)/2]
	if block.Timestamp <= medianTimestamp {
		return fmt.Errorf("timestamp is not greater than median time past (Current: %d, Median: %d)", block.Timestamp, medianTimestamp)
	}

	// 2. Bound timestamp advances using only the chain, never the local clock.
	maxAdvance := int64(DriftTolerance / time.Second)
	maxTimestamp := prevBlock.Timestamp
	if maxTimestamp > math.MaxInt64-maxAdvance {
		maxTimestamp = math.MaxInt64
	} else {
		maxTimestamp += maxAdvance
	}
	if block.Timestamp > maxTimestamp {
		return fmt.Errorf("timestamp advances too far beyond parent (Current: %d, Parent: %d, Maximum advance: %d)", block.Timestamp, prevBlock.Timestamp, maxAdvance)
	}

	// 3. Anti-Spam (Proof of Work) — checked against the verified hash.
	if !CheckProofOfWork(block.Hash, block.Difficulty) {
		return fmt.Errorf("invalid PoA Proof-of-Work (Hash: %x)", block.Hash)
	}

	return nil
}
