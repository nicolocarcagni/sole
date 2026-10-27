package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestNewMerkleTree_EmptyData(t *testing.T) {
	// Should not panic, return tree with RootNode nil
	tree1 := NewMerkleTree(nil)
	if tree1.RootNode != nil {
		t.Errorf("Expected nil root node for nil input")
	}

	tree2 := NewMerkleTree([][]byte{})
	if tree2.RootNode != nil {
		t.Errorf("Expected nil root node for empty slice input")
	}

	// GetMerklePath on empty tree
	_, err := tree2.GetMerklePath([]byte("non-existent"))
	if err == nil || err.Error() != "merkle tree is empty" {
		t.Errorf("Expected 'merkle tree is empty' error, got: %v", err)
	}
}

func TestNewMerkleTree_SingleLeaf(t *testing.T) {
	data := [][]byte{[]byte("tx1")}
	tree := NewMerkleTree(data)

	if tree.RootNode == nil {
		t.Fatalf("Expected non-nil root node")
	}

	hash1 := sha256.Sum256(data[0])
	expectedRootHash := sha256.Sum256(append(hash1[:], hash1[:]...))

	if !bytes.Equal(tree.RootNode.Data, expectedRootHash[:]) {
		t.Errorf("Root node mismatch for single leaf")
	}
}

func TestNewMerkleTree_OddCount(t *testing.T) {
	data := [][]byte{[]byte("tx1"), []byte("tx2"), []byte("tx3")}
	tree := NewMerkleTree(data)

	if tree.RootNode == nil {
		t.Fatalf("Expected non-nil root node")
	}

	hash1 := sha256.Sum256(data[0])
	hash2 := sha256.Sum256(data[1])
	hash3 := sha256.Sum256(data[2])

	hash12 := sha256.Sum256(append(hash1[:], hash2[:]...))
	hash33 := sha256.Sum256(append(hash3[:], hash3[:]...)) // tx3 duplicated
	expectedRoot := sha256.Sum256(append(hash12[:], hash33[:]...))

	if !bytes.Equal(tree.RootNode.Data, expectedRoot[:]) {
		t.Errorf("Root node mismatch for odd count tree")
	}
}

func TestNewMerkleTree_EvenCount(t *testing.T) {
	// Power of 2 (4 leaves)
	data4 := [][]byte{[]byte("tx1"), []byte("tx2"), []byte("tx3"), []byte("tx4")}
	tree4 := NewMerkleTree(data4)

	h1 := sha256.Sum256(data4[0])
	h2 := sha256.Sum256(data4[1])
	h3 := sha256.Sum256(data4[2])
	h4 := sha256.Sum256(data4[3])

	h12 := sha256.Sum256(append(h1[:], h2[:]...))
	h34 := sha256.Sum256(append(h3[:], h4[:]...))
	exp4 := sha256.Sum256(append(h12[:], h34[:]...))

	if !bytes.Equal(tree4.RootNode.Data, exp4[:]) {
		t.Errorf("Root mismatch for 4 leaves")
	}

	// 2 leaves
	data2 := [][]byte{[]byte("tx1"), []byte("tx2")}
	tree2 := NewMerkleTree(data2)

	h2_12 := sha256.Sum256(append(h1[:], h2[:]...))
	if !bytes.Equal(tree2.RootNode.Data, h2_12[:]) {
		t.Errorf("Root mismatch for 2 leaves")
	}
}

func TestGetMerklePath(t *testing.T) {
	data := [][]byte{[]byte("tx1"), []byte("tx2"), []byte("tx3"), []byte("tx4")}
	tree := NewMerkleTree(data)

	// Retrieve path for each tx and verify
	for _, txID := range data {
		path, err := tree.GetMerklePath(txID)
		if err != nil {
			t.Fatalf("Failed to get merkle path for %s: %v", string(txID), err)
		}

		// Reconstruct root
		currentHash := sha256.Sum256(txID)
		currentHashSlice := currentHash[:]

		for _, step := range path {
			stepHashBytes, _ := hex.DecodeString(step.Hash)
			if step.Direction == "L" {
				newHash := sha256.Sum256(append(stepHashBytes, currentHashSlice...))
				currentHashSlice = newHash[:]
			} else { // "R"
				newHash := sha256.Sum256(append(currentHashSlice, stepHashBytes...))
				currentHashSlice = newHash[:]
			}
		}

		if !bytes.Equal(currentHashSlice, tree.RootNode.Data) {
			t.Errorf("Reconstructed root hash does not match tree root for tx %s", string(txID))
		}
	}

	// Non-existent transaction
	_, err := tree.GetMerklePath([]byte("unknown-tx"))
	if err == nil || err.Error() != "transaction not found in merkle tree" {
		t.Errorf("Expected 'transaction not found in merkle tree', got: %v", err)
	}
}

func TestNewMerkleTreeDoesNotMutateInputWithSpareCapacity(t *testing.T) {
	// Allocate a slice with capacity larger than length and an odd number of elements.
	// Index 3 is a sentinel value in the caller's backing array.
	backing := make([][]byte, 4)
	backing[0] = []byte("tx1")
	backing[1] = []byte("tx2")
	backing[2] = []byte("tx3")
	sentinel := []byte("sentinel-do-not-overwrite")
	backing[3] = sentinel

	// Pass a slice of length 3 (odd) with capacity 4
	input := backing[:3]
	_ = NewMerkleTree(input)

	// If NewMerkleTree appends directly to the input slice, it will overwrite backing[3] with "tx3"
	if !bytes.Equal(backing[3], sentinel) {
		t.Fatalf("NewMerkleTree mutated caller's underlying slice capacity: expected %q, got %q", sentinel, backing[3])
	}
	if len(input) != 3 {
		t.Fatalf("expected input length to remain 3, got %d", len(input))
	}
	if !bytes.Equal(input[0], []byte("tx1")) || !bytes.Equal(input[1], []byte("tx2")) || !bytes.Equal(input[2], []byte("tx3")) {
		t.Fatalf("NewMerkleTree mutated input slice elements")
	}
}

func TestMerkleOddLeaves(t *testing.T) {
	tests := []struct {
		name   string
		leaves [][]byte
	}{
		{
			name:   "1 leaf",
			leaves: [][]byte{[]byte("tx1")},
		},
		{
			name:   "3 leaves",
			leaves: [][]byte{[]byte("tx1"), []byte("tx2"), []byte("tx3")},
		},
		{
			name:   "5 leaves",
			leaves: [][]byte{[]byte("tx1"), []byte("tx2"), []byte("tx3"), []byte("tx4"), []byte("tx5")},
		},
		{
			name:   "7 leaves",
			leaves: [][]byte{[]byte("tx1"), []byte("tx2"), []byte("tx3"), []byte("tx4"), []byte("tx5"), []byte("tx6"), []byte("tx7")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalLeaves := make([][]byte, len(tt.leaves))
			for i, leaf := range tt.leaves {
				originalLeaves[i] = append([]byte(nil), leaf...)
			}

			tree := NewMerkleTree(tt.leaves)
			if tree == nil || tree.RootNode == nil {
				t.Fatalf("expected non-nil tree and root node for %s", tt.name)
			}

			// Verify caller's input slice was not modified
			if len(tt.leaves) != len(originalLeaves) {
				t.Fatalf("input length modified: expected %d, got %d", len(originalLeaves), len(tt.leaves))
			}
			for i := range tt.leaves {
				if !bytes.Equal(tt.leaves[i], originalLeaves[i]) {
					t.Fatalf("leaf %d modified in caller's slice", i)
				}
			}

			// Verify GetMerklePath produces valid proofs for all leaves
			for _, leaf := range tt.leaves {
				path, err := tree.GetMerklePath(leaf)
				if err != nil {
					t.Fatalf("GetMerklePath failed for leaf %s: %v", string(leaf), err)
				}

				current := sha256.Sum256(leaf)
				currentSlice := current[:]
				for _, step := range path {
					stepBytes, err := hex.DecodeString(step.Hash)
					if err != nil {
						t.Fatalf("invalid hex in proof step: %v", err)
					}
					if step.Direction == "L" {
						h := sha256.Sum256(append(stepBytes, currentSlice...))
						currentSlice = h[:]
					} else {
						h := sha256.Sum256(append(currentSlice, stepBytes...))
						currentSlice = h[:]
					}
				}

				if !bytes.Equal(currentSlice, tree.RootNode.Data) {
					t.Fatalf("merkle path proof failed for leaf %s in %s", string(leaf), tt.name)
				}
			}
		})
	}
}
