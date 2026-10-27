package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/nicolocarcagni/sole/pkg/core"
)

func waitForCondition(t *testing.T, cond func() bool, timeout time.Duration, desc string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for condition: %s", desc)
}

func TestWebSocketRegistersWithMempoolHub(t *testing.T) {
	rs := NewRestServer(nil)
	defer rs.Stop()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()

	go func() {
		_ = rs.Serve(l)
	}()

	wsURL := fmt.Sprintf("ws://%s/ws/mempool", l.Addr().String())
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()

	// Wait for client to be registered in MempoolHub
	waitForCondition(t, func() bool {
		return rs.MempoolHub.ClientCount() == 1
	}, 2*time.Second, "mempool client registration")

	// Broadcast transaction to MempoolHub
	tx := &core.Transaction{
		ID:        []byte("test-tx-id"),
		Timestamp: time.Now().Unix(),
		Vin:       []core.TxInput{},
		Vout:      []core.TxOutput{},
	}
	BroadcastMempoolTx(rs.MempoolHub, tx)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read websocket message: %v", err)
	}

	var evt WsMempoolEvent
	if err := json.Unmarshal(msg, &evt); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}
	if evt.Event != "new_tx" {
		t.Fatalf("expected event 'new_tx', got %s", evt.Event)
	}
	if evt.TxID != hex.EncodeToString(tx.ID) {
		t.Fatalf("expected txid %s, got %s", hex.EncodeToString(tx.ID), evt.TxID)
	}

	// Close client connection and verify deregistration
	_ = conn.Close()
	waitForCondition(t, func() bool {
		return rs.MempoolHub.ClientCount() == 0
	}, 2*time.Second, "mempool client deregistration")
}

func TestWebSocketRegistersWithBlockHub(t *testing.T) {
	rs := NewRestServer(nil)
	defer rs.Stop()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()

	go func() {
		_ = rs.Serve(l)
	}()

	wsURL := fmt.Sprintf("ws://%s/ws/blocks", l.Addr().String())
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()

	// Wait for client to be registered in BlockHub
	waitForCondition(t, func() bool {
		return rs.BlockHub.ClientCount() == 1
	}, 2*time.Second, "block client registration")

	// Broadcast block to BlockHub
	block := &core.Block{
		Hash:         []byte("test-block-hash"),
		Height:       42,
		Transactions: []*core.Transaction{},
	}
	BroadcastBlock(rs.BlockHub, block)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read websocket message: %v", err)
	}

	var evt WsBlockEvent
	if err := json.Unmarshal(msg, &evt); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}
	if evt.Event != "new_block" {
		t.Fatalf("expected event 'new_block', got %s", evt.Event)
	}
	if evt.Hash != hex.EncodeToString(block.Hash) {
		t.Fatalf("expected hash %s, got %s", hex.EncodeToString(block.Hash), evt.Hash)
	}
	if evt.Height != block.Height {
		t.Fatalf("expected height %d, got %d", block.Height, evt.Height)
	}

	// Close client connection and verify deregistration
	_ = conn.Close()
	waitForCondition(t, func() bool {
		return rs.BlockHub.ClientCount() == 0
	}, 2*time.Second, "block client deregistration")
}

func TestWebSocketServerShutdown(t *testing.T) {
	rs := NewRestServer(nil)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- rs.Serve(l)
	}()

	addr := l.Addr().String()
	mempoolURL := fmt.Sprintf("ws://%s/ws/mempool", addr)
	blockURL := fmt.Sprintf("ws://%s/ws/blocks", addr)

	mempoolConn, _, err := websocket.DefaultDialer.Dial(mempoolURL, nil)
	if err != nil {
		t.Fatalf("failed to dial mempool ws: %v", err)
	}
	defer mempoolConn.Close()

	blockConn, _, err := websocket.DefaultDialer.Dial(blockURL, nil)
	if err != nil {
		t.Fatalf("failed to dial block ws: %v", err)
	}
	defer blockConn.Close()

	waitForCondition(t, func() bool {
		return rs.MempoolHub.ClientCount() == 1 && rs.BlockHub.ClientCount() == 1
	}, 2*time.Second, "both clients registered")

	// Trigger graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := rs.Shutdown(ctx); err != nil {
		t.Fatalf("rs.Shutdown returned error: %v", err)
	}

	// Verify server terminated
	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatalf("server exited with unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server to exit")
	}

	// Verify clients observe connection closure
	_ = mempoolConn.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := mempoolConn.ReadMessage(); err == nil {
		t.Fatal("expected error reading from closed mempool connection, got nil")
	}

	_ = blockConn.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := blockConn.ReadMessage(); err == nil {
		t.Fatal("expected error reading from closed block connection, got nil")
	}

	// Verify hubs have 0 clients
	waitForCondition(t, func() bool {
		return rs.MempoolHub.ClientCount() == 0 && rs.BlockHub.ClientCount() == 0
	}, 2*time.Second, "both hubs client count 0")
}

func TestWebSocketNilHubReturnsServiceUnavailable(t *testing.T) {
	// Directly call handleWs with nil hub
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ws/mempool", nil)
	handleWs(nil, rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected HTTP 503 for nil hub, got %d", rec.Code)
	}

	// With uninitialized hub (Register channel is nil)
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/ws/mempool", nil)
	handleWs(&EventHub{}, rec2, req2)

	if rec2.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected HTTP 503 for uninitialized hub, got %d", rec2.Code)
	}
}
