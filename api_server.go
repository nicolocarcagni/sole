package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

// RestServer represents the HTTP API Server
type RestServer struct {
	P2P *Server
}

// StartRestServer starts the API server on the specified port
func StartRestServer(server *Server, port int) {
	rs := RestServer{P2P: server}

	router := mux.NewRouter()
	router.Use(commonMiddleware)

	// Endpoints
	router.HandleFunc("/balance/{address}", rs.getBalance).Methods("GET")
	router.HandleFunc("/blocks/tip", rs.getTip).Methods("GET")
	router.HandleFunc("/blocks/{hash}", rs.getBlock).Methods("GET")
	router.HandleFunc("/tx/send", rs.sendTx).Methods("POST")

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("🚀 API Server started on http://localhost%s\n", addr)

	srv := &http.Server{
		Handler:      router,
		Addr:         addr,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}

func commonMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*") // CORS
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		next.ServeHTTP(w, r)
	})
}

// Responses
type BalanceResponse struct {
	Address string `json:"address"`
	Balance int64  `json:"balance"`
}

type TipResponse struct {
	Height int    `json:"height"`
	Hash   string `json:"hash"`
}

type TxSendRequest struct {
	Hex string `json:"hex"` // Hex encoded transaction bytes
}

type SuccessResponse struct {
	Status string `json:"status"`
	TxID   string `json:"txid,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

// Handlers

func (rs *RestServer) getBalance(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	addr := vars["address"]

	if !ValidateAddress(addr) {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid address"})
		return
	}

	pubKeyHash, _ := Base58Decode([]byte(addr))
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4]

	utxos := rs.P2P.Blockchain.FindUnspentTransactions(pubKeyHash)
	balance := int64(0)

	for _, tx := range utxos {
		for _, out := range tx.Vout {
			if out.IsLockedWithKey(pubKeyHash) {
				balance += out.Value
			}
		}
	}

	json.NewEncoder(w).Encode(BalanceResponse{Address: addr, Balance: balance})
}

func (rs *RestServer) getTip(w http.ResponseWriter, r *http.Request) {
	height := rs.P2P.Blockchain.GetBestHeight()
	hash := rs.P2P.Blockchain.LastHash
	json.NewEncoder(w).Encode(TipResponse{Height: height, Hash: hex.EncodeToString(hash)})
}

func (rs *RestServer) getBlock(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hashHex := vars["hash"]

	hash, err := hex.DecodeString(hashHex)
	if err != nil {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid hash format"})
		return
	}

	block, err := rs.P2P.Blockchain.GetBlock(hash)
	if err != nil {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Block not found"})
		return
	}

	// We might want a custom JSON representation for Block if fields are private or complex
	// But Block fields are exported, so automatic JSON should work roughly ok
	json.NewEncoder(w).Encode(block)
}

func (rs *RestServer) sendTx(w http.ResponseWriter, r *http.Request) {
	var req TxSendRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	txBytes, err := hex.DecodeString(req.Hex)
	if err != nil {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid hex"})
		return
	}

	// Deserialize
	tx := DeserializeTransaction(txBytes)

	// Basic Validation (Proof of concept)
	// In production, we'd verify signatures and UTXOs more strictly before mempool
	if rs.P2P.Blockchain.VerifyTransaction(&tx) == false {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Transaction invalid"})
		return
	}

	txID := hex.EncodeToString(tx.ID)

	// Add to Mempool
	if rs.P2P.Mempool[txID].ID == nil {
		rs.P2P.Mempool[txID] = tx
		fmt.Printf("API: Transazione aggiunta alla Mempool: %s\n", txID)

		// Broadcast Inv
		peers := rs.P2P.Host.Network().Peers()
		for _, p := range peers {
			rs.P2P.SendInv(p, "tx", [][]byte{tx.ID})
		}

		json.NewEncoder(w).Encode(SuccessResponse{Status: "success", TxID: txID})
	} else {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Transaction already in mempool or exists"})
	}
}
