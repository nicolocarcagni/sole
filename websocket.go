package main

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type EventHub struct {
	clients    map[chan []byte]bool
	Register   chan chan []byte
	Unregister chan chan []byte
	Broadcast  chan []byte
	mux        sync.Mutex
}

func NewEventHub() *EventHub {
	return &EventHub{
		clients:    make(map[chan []byte]bool),
		Register:   make(chan chan []byte),
		Unregister: make(chan chan []byte),
		Broadcast:  make(chan []byte, 256),
	}
}

func (h *EventHub) Run() {
	for {
		select {
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

func handleWs(hub *EventHub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("⚠️  [WS] Upgrade failed: %v", err)
		return
	}

	send := make(chan []byte, 64)
	hub.Register <- send

	defer func() {
		hub.Unregister <- send
		conn.Close()
	}()

	// Writer goroutine: relay hub broadcasts to this WebSocket connection
	go func() {
		for msg := range send {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}()

	// Reader loop: keep the connection alive, discard incoming messages
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// JSON payloads

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

// BroadcastMempoolTx builds and sends a mempool event (non-blocking)
func BroadcastMempoolTx(hub *EventHub, tx *Transaction) {
	if hub == nil {
		return
	}

	var inputs []WsInput
	if tx.IsCoinbase() {
		inputs = append(inputs, WsInput{Address: "COINBASE"})
	} else {
		for _, vin := range tx.Vin {
			inputs = append(inputs, WsInput{
				Address: PubKeyToAddress(vin.PubKey),
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
				Address: PubKeyHashToAddress(vout.PubKeyHash),
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
	default:
	}
}

// BroadcastBlock builds and sends a block event (non-blocking)
func BroadcastBlock(hub *EventHub, block *Block) {
	if hub == nil {
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
				Address: PubKeyToAddress(vin.PubKey),
			})
		}

		var outputs []WsOutput
		for _, vout := range tx.Vout {
			if vout.IsOPReturn() {
				continue
			}
			if vout.Value > 0 {
				outputs = append(outputs, WsOutput{
					Address: PubKeyHashToAddress(vout.PubKeyHash),
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
	default:
	}
}


