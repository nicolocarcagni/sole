package p2p

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/gob"
	"encoding/hex"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/nicolocarcagni/sole/pkg/consensus"
	"github.com/nicolocarcagni/sole/pkg/core"
	"github.com/nicolocarcagni/sole/pkg/storage"
)

func TestHandleBlockPropagatesUTXOError(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	validator := append(key.PublicKey.X.FillBytes(make([]byte, 32)), key.PublicKey.Y.FillBytes(make([]byte, 32))...)
	authorizedValidator := append([]byte{0x04}, validator...)
	oldValidators := append([]string(nil), consensus.AuthorizedValidators...)
	consensus.AuthorizedValidators = append(consensus.AuthorizedValidators, hex.EncodeToString(authorizedValidator))
	t.Cleanup(func() { consensus.AuthorizedValidators = oldValidators })

	dbPath, err := os.MkdirTemp(t.TempDir(), "blockchain-")
	if err != nil {
		t.Fatal(err)
	}
	db, err := badger.Open(badger.DefaultOptions(dbPath).WithLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	genesis, err := storage.NewGenesisBlock()
	if err != nil {
		t.Fatal(err)
	}
	genesisData, err := genesis.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(txn *badger.Txn) error {
		if err := txn.Set(genesis.Hash, genesisData); err != nil {
			return err
		}
		return txn.Set([]byte("lh"), genesis.Hash)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	readOnlyOptions := badger.DefaultOptions(dbPath).WithLogger(nil)
	readOnlyOptions.ReadOnly = true
	db, err = badger.Open(readOnlyOptions)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	chain := &storage.Blockchain{LastHash: append([]byte(nil), genesis.Hash...), Database: db}
	block := core.NewBlock(nil, genesis.Hash, genesis.Height+1, validator)
	block.Timestamp = genesis.Timestamp + 1
	consensus.MineBlock(block)
	if err := consensus.SignBlock(block, *key); err != nil {
		t.Fatal(err)
	}
	serializedBlock, err := block.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	var request bytes.Buffer
	if err := gob.NewEncoder(&request).Encode(BlockMsg{AddrFrom: "test", Block: serializedBlock}); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		Blockchain: chain,
		UTXOSet:    &storage.UTXOSet{Blockchain: chain},
		Mempool:    make(map[string]core.MempoolItem),
	}
	if err := server.HandleBlock(request.Bytes(), peer.ID("test-peer")); err == nil {
		t.Fatal("HandleBlock did not propagate the read-only database write error")
	}
}

func TestStartupRecoversInterruptedUTXOUpdate(t *testing.T) {
	dbPath := t.TempDir()
	db, err := badger.Open(badger.DefaultOptions(dbPath).WithLogger(nil))
	if err != nil {
		t.Fatal(err)
	}

	genesis := &core.Block{Hash: []byte("genesis"), Height: 0}
	tx := &core.Transaction{
		ID:   []byte("canonical-tx"),
		Vin:  []core.TxInput{{Txid: []byte{}, Vout: -1, PubKey: []byte("coinbase")}},
		Vout: []core.TxOutput{{Value: 10, PubKeyHash: []byte("canonical-owner")}},
	}
	block := &core.Block{
		Hash:          []byte("tip"),
		Height:        1,
		PrevBlockHash: genesis.Hash,
		Transactions:  []*core.Transaction{tx},
	}
	genesisData, err := genesis.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	blockData, err := block.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	staleKey := []byte(storage.UtxoPrefix + "stale-output-0")
	staleOutput, err := storage.SerializeUTXO(core.TxOutput{Value: 99, PubKeyHash: []byte("stale-owner")})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(txn *badger.Txn) error {
		if err := txn.Set(genesis.Hash, genesisData); err != nil {
			return err
		}
		if err := txn.Set(block.Hash, blockData); err != nil {
			return err
		}
		if err := txn.Set([]byte("lh"), block.Hash); err != nil {
			return err
		}
		return txn.Set(staleKey, staleOutput)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen after the simulated crash with a stale derived index.
	db, err = badger.Open(badger.DefaultOptions(dbPath).WithLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	chain := &storage.Blockchain{LastHash: append([]byte(nil), block.Hash...), Database: db}
	if err := recoverUTXOSet(&storage.UTXOSet{Blockchain: chain}); err != nil {
		t.Fatalf("startup recovery failed: %v", err)
	}

	_, canonical, err := (&storage.UTXOSet{Blockchain: chain}).FindSpendableOutputs([]byte("canonical-owner"), 99)
	if err != nil {
		t.Fatal(err)
	}
	if len(canonical[hex.EncodeToString(tx.ID)]) != 1 {
		t.Fatal("canonical UTXO was not reconstructed")
	}
	_, stale, err := (&storage.UTXOSet{Blockchain: chain}).FindSpendableOutputs([]byte("stale-owner"), 99)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 0 {
		t.Fatal("stale UTXO remains after recovery")
	}
}

type mockConn struct {
	network.Conn
	remotePeer peer.ID
}

func (c *mockConn) RemotePeer() peer.ID {
	return c.remotePeer
}

type mockStream struct {
	network.Stream
	conn      network.Conn
	pipeConn  net.Conn
	resetCh   chan struct{}
	closeCh   chan struct{}
	resetOnce sync.Once
	closeOnce sync.Once
}

func newMockStream(peerID peer.ID, pipeConn net.Conn) *mockStream {
	return &mockStream{
		conn:     &mockConn{remotePeer: peerID},
		pipeConn: pipeConn,
		resetCh:  make(chan struct{}),
		closeCh:  make(chan struct{}),
	}
}

func (m *mockStream) Conn() network.Conn {
	return m.conn
}

func (m *mockStream) Read(p []byte) (int, error) {
	return m.pipeConn.Read(p)
}

func (m *mockStream) Write(p []byte) (int, error) {
	return m.pipeConn.Write(p)
}

func (m *mockStream) Close() error {
	m.closeOnce.Do(func() {
		close(m.closeCh)
	})
	return m.pipeConn.Close()
}

func (m *mockStream) Reset() error {
	m.resetOnce.Do(func() {
		close(m.resetCh)
	})
	return m.pipeConn.Close()
}

func (m *mockStream) SetReadDeadline(t time.Time) error {
	return m.pipeConn.SetReadDeadline(t)
}

func (m *mockStream) isReset() bool {
	select {
	case <-m.resetCh:
		return true
	default:
		return false
	}
}

func (m *mockStream) isClosed() bool {
	select {
	case <-m.closeCh:
		return true
	default:
		return false
	}
}

func TestHandleStreamCapsConcurrentStreams(t *testing.T) {
	server := &Server{
		MaxConcurrentStreams:     2,
		MaxPeerConcurrentStreams: 1,
	}
	defer server.Stop()

	peerA := peer.ID("peer-A")
	peerB := peer.ID("peer-B")
	peerC := peer.ID("peer-C")

	// Peer A opens first stream -> should be accepted
	cA1, sA1 := net.Pipe()
	defer cA1.Close()
	streamA1 := newMockStream(peerA, sA1)
	server.HandleStream(streamA1)

	// Peer A opens second stream -> exceeds per-peer cap (1), should be rejected immediately
	cA2, sA2 := net.Pipe()
	defer cA2.Close()
	streamA2 := newMockStream(peerA, sA2)
	server.HandleStream(streamA2)

	if !streamA2.isReset() {
		t.Fatal("expected second stream from peer-A to be reset due to per-peer limit")
	}

	// Peer B opens stream -> should be accepted (active: 2, peerB: 1)
	cB1, sB1 := net.Pipe()
	defer cB1.Close()
	streamB1 := newMockStream(peerB, sB1)
	server.HandleStream(streamB1)

	if streamA1.isReset() || streamB1.isReset() {
		t.Fatal("valid streams should not be reset")
	}

	// Peer C opens stream -> exceeds global cap (2), should be rejected immediately
	cC1, sC1 := net.Pipe()
	defer cC1.Close()
	streamC1 := newMockStream(peerC, sC1)
	server.HandleStream(streamC1)

	if !streamC1.isReset() {
		t.Fatal("expected stream from peer-C to be reset due to global stream limit")
	}

	if server.ActiveStreams() != 2 {
		t.Fatalf("expected 2 active streams, got %d", server.ActiveStreams())
	}
	if server.PeerStreams(peerA) != 1 {
		t.Fatalf("expected 1 active stream for peer-A, got %d", server.PeerStreams(peerA))
	}
	if server.PeerStreams(peerB) != 1 {
		t.Fatalf("expected 1 active stream for peer-B, got %d", server.PeerStreams(peerB))
	}

	// Close streamA1 and verify permit is freed, allowing peerC to connect
	_ = cA1.Close()
	deadline := time.Now().Add(2 * time.Second)
	for server.PeerStreams(peerA) > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if server.PeerStreams(peerA) != 0 {
		t.Fatalf("expected peer-A streams to drop to 0, got %d", server.PeerStreams(peerA))
	}

	cC2, sC2 := net.Pipe()
	defer cC2.Close()
	streamC2 := newMockStream(peerC, sC2)
	server.HandleStream(streamC2)

	if streamC2.isReset() {
		t.Fatal("expected peer-C stream to be accepted after peer-A permit freed")
	}
	if server.PeerStreams(peerC) != 1 {
		t.Fatalf("expected 1 active stream for peer-C, got %d", server.PeerStreams(peerC))
	}
}

func TestHandleStreamCancelsOnShutdown(t *testing.T) {
	server := &Server{
		MaxConcurrentStreams:     2,
		MaxPeerConcurrentStreams: 2,
	}
	defer server.Stop()

	peerA := peer.ID("peer-A")
	cA, sA := net.Pipe()
	defer cA.Close()
	streamA := newMockStream(peerA, sA)

	server.HandleStream(streamA)

	if server.ActiveStreams() != 1 {
		t.Fatalf("expected 1 active stream, got %d", server.ActiveStreams())
	}

	// Shutdown server
	server.Stop()

	if !streamA.isReset() {
		t.Fatal("expected active stream to be reset on server shutdown")
	}

	if server.ActiveStreams() != 0 {
		t.Fatalf("expected 0 active streams after shutdown, got %d", server.ActiveStreams())
	}

	// New streams arriving after shutdown must be immediately rejected
	cA2, sA2 := net.Pipe()
	defer cA2.Close()
	streamA2 := newMockStream(peerA, sA2)
	server.HandleStream(streamA2)

	if !streamA2.isReset() {
		t.Fatal("expected stream arriving after shutdown to be reset immediately")
	}

	// Start called after Stop should return immediately without hanging
	startDone := make(chan struct{})
	go func() {
		server.Start()
		close(startDone)
	}()
	select {
	case <-startDone:
	case <-time.After(1 * time.Second):
		t.Fatal("server.Start() hanged after server.Stop()")
	}
}

func TestHandleStreamReleasesPermitAfterTimeout(t *testing.T) {
	server := &Server{
		MaxConcurrentStreams:     1,
		MaxPeerConcurrentStreams: 1,
		StreamTimeout:            50 * time.Millisecond,
	}
	defer server.Stop()

	peerA := peer.ID("peer-A")
	cA1, sA1 := net.Pipe()
	defer cA1.Close()
	streamA1 := newMockStream(peerA, sA1)

	// Stream starts reading, client sends no data, so it hits timeout
	server.HandleStream(streamA1)

	if server.ActiveStreams() != 1 {
		t.Fatalf("expected 1 active stream, got %d", server.ActiveStreams())
	}

	// Wait for read timeout to trigger permit release
	deadline := time.Now().Add(2 * time.Second)
	for server.ActiveStreams() > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if server.ActiveStreams() != 0 {
		t.Fatalf("expected permit to be released after timeout, active: %d", server.ActiveStreams())
	}

	// Subsequent stream should now succeed in acquiring the permit
	cA2, sA2 := net.Pipe()
	defer cA2.Close()
	streamA2 := newMockStream(peerA, sA2)
	server.HandleStream(streamA2)

	if streamA2.isReset() {
		t.Fatal("subsequent stream should have acquired the released permit")
	}
	if server.ActiveStreams() != 1 {
		t.Fatalf("expected 1 active stream for second stream, got %d", server.ActiveStreams())
	}
}
