package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// MerkleTree represents a Merkle tree
type MerkleTree struct {
	RootNode *MerkleNode
}

// MerkleNode represents a Merkle tree node
type MerkleNode struct {
	Left  *MerkleNode
	Right *MerkleNode
	Data  []byte
}

// NewMerkleTree creates a new Merkle tree from a sequence of data
func NewMerkleTree(data [][]byte) *MerkleTree {
	var nodes []MerkleNode

	if len(data)%2 != 0 {
		data = append(data, data[len(data)-1])
	}

	for _, datum := range data {
		node := NewMerkleNode(nil, nil, datum)
		nodes = append(nodes, *node)
	}

	// Recursively build tree levels
	for len(nodes) > 1 {
		// If odd number of nodes, duplicate last one
		if len(nodes)%2 != 0 {
			nodes = append(nodes, nodes[len(nodes)-1])
		}

		var newLevel []MerkleNode
		for j := 0; j < len(nodes); j += 2 {
			node := NewMerkleNode(&nodes[j], &nodes[j+1], nil)
			newLevel = append(newLevel, *node)
		}
		nodes = newLevel
	}

	mTree := MerkleTree{&nodes[0]}

	return &mTree
}

// NewMerkleNode creates a new Merkle tree node
func NewMerkleNode(left, right *MerkleNode, data []byte) *MerkleNode {
	mNode := MerkleNode{}

	if left == nil && right == nil {
		hash := sha256.Sum256(data)
		mNode.Data = hash[:]
	} else {
		prevHashes := append(left.Data, right.Data...)
		hash := sha256.Sum256(prevHashes)
		mNode.Data = hash[:]
	}

	mNode.Left = left
	mNode.Right = right

	return &mNode
}

// MerkleStep represents a node in the proof path
type MerkleStep struct {
	Hash      string `json:"hash"`
	Direction string `json:"direction"` // "L" (Left) or "R" (Right)
}

// GetMerklePath extracts the Merkle Proof path for a target transaction ID
func (m *MerkleTree) GetMerklePath(txID []byte) ([]MerkleStep, error) {
	if m.RootNode == nil {
		return nil, fmt.Errorf("merkle tree is empty")
	}

	targetHashBytes := sha256.Sum256(txID)
	
	path, found := m.RootNode.findPath(targetHashBytes[:])
	if !found {
		return nil, fmt.Errorf("transaction not found in merkle tree")
	}

	return path, nil
}

// findPath recursively traverses the tree to find the target leaf and builds the path
func (n *MerkleNode) findPath(targetHash []byte) ([]MerkleStep, bool) {
	// Base Case: Leaf Node
	if n.Left == nil && n.Right == nil {
		if bytes.Equal(n.Data, targetHash) {
			return []MerkleStep{}, true
		}
		return nil, false
	}

	// Search Left Branch
	if n.Left != nil {
		path, found := n.Left.findPath(targetHash)
		if found {
			// Sibling is the Right node
			step := MerkleStep{
				Hash:      hex.EncodeToString(n.Right.Data),
				Direction: "R",
			}
			return append(path, step), true
		}
	}

	// Search Right Branch
	if n.Right != nil {
		path, found := n.Right.findPath(targetHash)
		if found {
			// Sibling is the Left node
			step := MerkleStep{
				Hash:      hex.EncodeToString(n.Left.Data),
				Direction: "L",
			}
			return append(path, step), true
		}
	}

	return nil, false
}
