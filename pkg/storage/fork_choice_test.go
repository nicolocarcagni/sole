package storage

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/nicolocarcagni/sole/pkg/consensus"
	"github.com/nicolocarcagni/sole/pkg/core"
)

var forkChoiceTimestampOffset int64

func TestForkChoiceUsesCumulativeWork(t *testing.T) {
	key := forkChoiceValidator(t)
	chain := newTestBlockchain(t)
	genesis := forkChoiceGenesis(t, chain)
	utxo := &UTXOSet{Blockchain: chain}
	if err := utxo.Reindex(); err != nil {
		t.Fatalf("initialize UTXO set: %v", err)
	}

	lowWorkFirst := makeForkChoiceBlock(t, key, genesis, 1, 1)
	if added, err := chain.AddBlockWithUTXO(lowWorkFirst, utxo); err != nil || !added {
		t.Fatalf("add first low-work block: added=%v err=%v", added, err)
	}
	lowWorkSecond := makeForkChoiceBlock(t, key, lowWorkFirst, 1, 1)
	if added, err := chain.AddBlockWithUTXO(lowWorkSecond, utxo); err != nil || !added {
		t.Fatalf("add second low-work block: added=%v err=%v", added, err)
	}

	shortHighWork := makeForkChoiceBlock(t, key, genesis, 2, 2)
	if added, err := chain.AddBlockWithUTXO(shortHighWork, utxo); err != nil || !added {
		t.Fatalf("add shorter high-work block: added=%v err=%v", added, err)
	}

	if got := chain.GetLastHash(); !bytes.Equal(got, shortHighWork.Hash) {
		t.Fatalf("canonical tip = %x, want shorter chain with greater cumulative work %x", got, shortHighWork.Hash)
	}
}

func TestForkChoiceEqualWorkTieBreakDeterministic(t *testing.T) {
	key := forkChoiceValidator(t)
	parent := makeForkChoiceGenesis()
	first := makeForkChoiceBlock(t, key, parent, 1, 1)
	second := makeForkChoiceBlock(t, key, parent, 1, 1)
	want := first.Hash
	if bytes.Compare(second.Hash, want) < 0 {
		want = second.Hash
	}

	for _, order := range [][]*core.Block{{first, second}, {second, first}} {
		chain := newTestBlockchain(t)
		forkChoiceStoreGenesis(t, chain, parent)
		for _, block := range order {
			if added, err := chain.AddBlock(block); err != nil || !added {
				t.Fatalf("add equal-work block: added=%v err=%v", added, err)
			}
		}
		if got := chain.GetLastHash(); !bytes.Equal(got, want) {
			t.Fatalf("canonical tip = %x, want deterministic lexicographic tie-break %x", got, want)
		}
	}
}

func TestNodesChooseSameChain(t *testing.T) {
	key := forkChoiceValidator(t)
	parent := makeForkChoiceGenesis()
	first := makeForkChoiceBlock(t, key, parent, 1, 1)
	second := makeForkChoiceBlock(t, key, parent, 1, 1)
	want := first.Hash
	if bytes.Compare(second.Hash, want) < 0 {
		want = second.Hash
	}

	for _, order := range [][]*core.Block{{first, second}, {second, first}} {
		chain := newTestBlockchain(t)
		forkChoiceStoreGenesis(t, chain, parent)
		for _, block := range order {
			if added, err := chain.AddBlock(block); err != nil || !added {
				t.Fatalf("add competing block: added=%v err=%v", added, err)
			}
		}
		if got := chain.GetLastHash(); !bytes.Equal(got, want) {
			t.Fatalf("node selected tip %x, want shared deterministic tip %x", got, want)
		}
	}
}

func makeForkChoiceGenesis() *core.Block {
	genesis := &core.Block{Timestamp: time.Now().Add(-time.Minute).Unix(), Height: 0}
	genesis.SetHash()
	return genesis
}

func forkChoiceValidator(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := append(key.PublicKey.X.FillBytes(make([]byte, 32)), key.PublicKey.Y.FillBytes(make([]byte, 32))...)
	encodedKey := hex.EncodeToString(append([]byte{0x04}, publicKey...))
	previous := append([]string(nil), consensus.AuthorizedValidators...)
	consensus.AuthorizedValidators = append(consensus.AuthorizedValidators, encodedKey)
	t.Cleanup(func() { consensus.AuthorizedValidators = previous })
	return key
}

func forkChoiceGenesis(t *testing.T, chain *Blockchain) *core.Block {
	t.Helper()
	genesis, err := NewGenesisBlock()
	if err != nil {
		t.Fatal(err)
	}
	forkChoiceStoreGenesis(t, chain, genesis)
	return genesis
}

func forkChoiceStoreGenesis(t *testing.T, chain *Blockchain, genesis *core.Block) {
	t.Helper()
	writeBlockToDB(t, chain.Database, genesis)
	chain.LastHash = append([]byte(nil), genesis.Hash...)
}

func makeForkChoiceBlock(t *testing.T, key *ecdsa.PrivateKey, parent *core.Block, height, difficulty uint8) *core.Block {
	t.Helper()
	forkChoiceTimestampOffset++
	validator := append(key.PublicKey.X.FillBytes(make([]byte, 32)), key.PublicKey.Y.FillBytes(make([]byte, 32))...)
	block := &core.Block{
		Timestamp:     parent.Timestamp + forkChoiceTimestampOffset,
		PrevBlockHash: append([]byte(nil), parent.Hash...),
		Height:        parent.Height + 1,
		Difficulty:    difficulty,
		Validator:     validator,
	}
	consensus.MineBlock(block)
	if err := consensus.SignBlock(block, *key); err != nil {
		t.Fatal(err)
	}
	return block
}
