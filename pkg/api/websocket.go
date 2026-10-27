package api

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/nicolocarcagni/sole/pkg/core"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type EventHub struct {
	clients    map[chan []byte]bool
	Register   chan chan []byte
	Unregister chan chan []byte
	Broadcast  chan []byte
	quit       chan struct{}
	done       chan struct{}
	started    atomic.Bool
	stopOnce   sync.Once
	mux        sync.RWMutex
}

func NewEventHub() *EventHub {
	return &EventHub{
		clients:    make(map[chan []byte]bool),
		Register:   make(chan chan []byte),
		Unregister: make(chan chan []byte),
		Broadcast:  make(chan []byte, 256),
		quit:       make(chan struct{}),
		done:       make(chan struct{}),
	}
}

func (h *EventHub) Run() {
	h.started.Store(true)
	defer close(h.done)
	for {
		select {
		case <-h.quit:
			h.mux.Lock()
			for client := range h.clients {
				delete(h.clients, client)
				close(client)
			}
			h.mux.Unlock()
			return

		case client := <-h.Register:
			h.mux.Lock()
			h.clients[client] = true
			h.mux.Unlock()

		case client := <-h.Unregister:
			h.mux.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client)
			}
			h.mux.Unlock()

		case message := <-h.Broadcast:
			h.mux.Lock()
			for client := range h.clients {
				select {
				case client <- message:
				default:
					delete(h.clients, client)
					close(client)
				}
			}
			h.mux.Unlock()
		}
	}
}

// Stop terminates the EventHub, closes all client channels, and waits for Run to exit.
func (h *EventHub) Stop() {
	if h == nil {
		return
	}
	h.stopOnce.Do(func() {
		close(h.quit)
	})
	if h.started.Load() {
		<-h.done
	}
}

// ClientCount returns the number of currently registered WebSocket client channels.
func (h *EventHub) ClientCount() int {
	if h == nil {
		return 0
	}
	h.mux.RLock()
	defer h.mux.RUnlock()
	return len(h.clients)
}

func handleWs(hub *EventHub, w http.ResponseWriter, r *http.Request) {
	if hub == nil || hub.Register == nil {
		http.Error(w, "WebSocket hub unavailable", http.StatusServiceUnavailable)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("⚠️  [WS] Upgrade failed: %v", err)
		return
	}

	send := make(chan []byte, 64)
	select {
	case hub.Register <- send:
	case <-hub.quit:
		_ = conn.Close()
		return
	}

	defer func() {
		select {
		case hub.Unregister <- send:
		case <-hub.quit:
		}
		_ = conn.Close()
	}()

	// Writer goroutine: relay hub broadcasts to this WebSocket connection
	go func() {
		for msg := range send {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
		// Send channel was closed (hub stopped or client unregistered)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
		_ = conn.Close()
	}()

	// Reader loop: keep the connection alive, discard incoming messages
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

type WsInput struct {
	Address string `json:"sender_address"`
}

type WsOutput struct {
	Address string `json:"address"`
	Value   int64  `json:"value"`
}

type WsMempoolEvent struct {
	Event   string     `json:"event"`
	TxID    string     `json:"txid"`
	Memo    string     `json:"memo,omitempty"`
	Inputs  []WsInput  `json:"inputs"`
	Outputs []WsOutput `json:"outputs"`
}

type WsBlockTxSummary struct {
	TxID    string     `json:"txid"`
	Inputs  []WsInput  `json:"inputs"`
	Outputs []WsOutput `json:"outputs"`
}

type WsBlockEvent struct {
	Event        string             `json:"event"`
	Hash         string             `json:"hash"`
	Height       int                `json:"height"`
	TxCount      int                `json:"tx_count"`
	Transactions []WsBlockTxSummary `json:"transactions"`
}

func BroadcastMempoolTx(hub *EventHub, tx *core.Transaction) {
	if hub == nil || hub.Broadcast == nil {
		return
	}

	var inputs []WsInput
	if tx.IsCoinbase() {
		inputs = append(inputs, WsInput{Address: "COINBASE"})
	} else {
		for _, vin := range tx.Vin {
			inputs = append(inputs, WsInput{
				Address: core.AddressFromPubKeyHash(core.HashPubKey(vin.PubKey)),
			})
		}
	}

	var outputs []WsOutput
	var memo string

	for _, vout := range tx.Vout {
		if vout.IsOPReturn() {
			memo = string(vout.PubKeyHash)
			continue
		}
		if vout.Value > 0 {
			outputs = append(outputs, WsOutput{
				Address: core.AddressFromPubKeyHash(vout.PubKeyHash),
				Value:   vout.Value,
			})
		}
	}

	evt := WsMempoolEvent{
		Event:   "new_tx",
		TxID:    hex.EncodeToString(tx.ID),
		Memo:    memo,
		Inputs:  inputs,
		Outputs: outputs,
	}

	payload, err := json.Marshal(evt)
	if err != nil {
		return
	}

	select {
	case hub.Broadcast <- payload:
	case <-hub.quit:
	default:
	}
}

func BroadcastBlock(hub *EventHub, block *core.Block) {
	if hub == nil || hub.Broadcast == nil {
		return
	}

	var txSummaries []WsBlockTxSummary
	for _, tx := range block.Transactions {
		if tx.IsCoinbase() {
			continue
		}

		var inputs []WsInput
		for _, vin := range tx.Vin {
			inputs = append(inputs, WsInput{
				Address: core.AddressFromPubKeyHash(core.HashPubKey(vin.PubKey)),
			})
		}

		var outputs []WsOutput
		for _, vout := range tx.Vout {
			if vout.IsOPReturn() {
				continue
			}
			if vout.Value > 0 {
				outputs = append(outputs, WsOutput{
					Address: core.AddressFromPubKeyHash(vout.PubKeyHash),
					Value:   vout.Value,
				})
			}
		}
		txSummaries = append(txSummaries, WsBlockTxSummary{
			TxID:    hex.EncodeToString(tx.ID),
			Inputs:  inputs,
			Outputs: outputs,
		})
	}

	evt := WsBlockEvent{
		Event:        "new_block",
		Hash:         hex.EncodeToString(block.Hash),
		Height:       block.Height,
		TxCount:      len(block.Transactions),
		Transactions: txSummaries,
	}

	payload, err := json.Marshal(evt)
	if err != nil {
		return
	}

	select {
	case hub.Broadcast <- payload:
	case <-hub.quit:
	default:
	}
}
