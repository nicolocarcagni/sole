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
func StartRestServer(server *Server, listenHost string, port int) {
	rs := RestServer{P2P: server}

	router := mux.NewRouter()
	router.Use(commonMiddleware)

	// Rate Limiters
	readLimiter := NewIPRateLimiter(20, 30) // 20 req/s, burst 30
	writeLimiter := NewIPRateLimiter(5, 10) // 5 req/s, burst 10

	// Middleware Wrappers
	readMW := RateLimitMiddleware(readLimiter)
	writeMW := RateLimitMiddleware(writeLimiter)

	// Endpoints (Applied specific rate limits)
	router.Handle("/balance/{address}", readMW(http.HandlerFunc(rs.getBalance))).Methods("GET")
	router.Handle("/utxos/{address}", readMW(http.HandlerFunc(rs.getUTXOs))).Methods("GET")
	router.Handle("/blocks/tip", readMW(http.HandlerFunc(rs.getTip))).Methods("GET")
	router.Handle("/blocks/{hash}", readMW(http.HandlerFunc(rs.getBlock))).Methods("GET")
	router.Handle("/transactions/{address}", readMW(http.HandlerFunc(rs.getTransactions))).Methods("GET")

	// Stricter limit for Sending Transactions
	router.Handle("/tx/send", writeMW(http.HandlerFunc(rs.sendTx))).Methods("POST")

	addr := fmt.Sprintf("%s:%d", listenHost, port)
	fmt.Printf("🚀 API Server started on http://%s\n", addr)

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

// JSON Response Structs
type JSONTransactionResponse struct {
	ID        string       `json:"id"`
	Inputs    []JSONInput  `json:"inputs"`
	Outputs   []JSONOutput `json:"outputs"`
	Timestamp int64        `json:"timestamp"` // Placeholder (block time if available, or 0)
}

type JSONInput struct {
	SenderAddress string `json:"sender"`
	Signature     string `json:"signature"`
}

type JSONOutput struct {
	ReceiverAddress string `json:"receiver"`
	Value           int64  `json:"value"`
}

// Helper: Convert PubKey to Address
func PubKeyToAddress(pubKey []byte) string {
	pubKeyHash := HashPubKey(pubKey)
	versionedPayload := append([]byte{version}, pubKeyHash...)
	checksum := checksum(versionedPayload)
	fullPayload := append(versionedPayload, checksum...)
	return string(Base58Encode(fullPayload))
}

// Helper: Convert PubKeyHash to Address
func PubKeyHashToAddress(pubKeyHash []byte) string {
	versionedPayload := append([]byte{version}, pubKeyHash...)
	checksum := checksum(versionedPayload)
	fullPayload := append(versionedPayload, checksum...)
	return string(Base58Encode(fullPayload))
}

// Mapper: ToJSONResponse
func ToJSONResponse(tx *Transaction) JSONTransactionResponse {
	var inputs []JSONInput
	var outputs []JSONOutput

	// Inputs
	if tx.IsCoinbase() {
		inputs = append(inputs, JSONInput{
			SenderAddress: "COINBASE",
			Signature:     "",
		})
	} else {
		for _, vin := range tx.Vin {
			inputs = append(inputs, JSONInput{
				SenderAddress: PubKeyToAddress(vin.PubKey),
				Signature:     hex.EncodeToString(vin.Signature),
			})
		}
	}

	// Outputs
	for _, vout := range tx.Vout {
		outputs = append(outputs, JSONOutput{
			ReceiverAddress: PubKeyHashToAddress(vout.PubKeyHash),
			Value:           vout.Value,
		})
	}

	return JSONTransactionResponse{
		ID:      hex.EncodeToString(tx.ID),
		Inputs:  inputs,
		Outputs: outputs,
	}
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

// UTXOResponse represents a spendable output
type UTXOResponse struct {
	TxID   string `json:"txid"`
	Vout   int    `json:"vout"`
	Amount int64  `json:"amount"`
}

func (rs *RestServer) getUTXOs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	addr := vars["address"]

	if !ValidateAddress(addr) {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid address"})
		return
	}

	pubKeyHash, _ := Base58Decode([]byte(addr))
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4]

	utxos := rs.P2P.Blockchain.FindUnspentTransactions(pubKeyHash)
	var response []UTXOResponse

	for _, tx := range utxos {
		for outIdx, out := range tx.Vout {
			if out.IsLockedWithKey(pubKeyHash) {
				response = append(response, UTXOResponse{
					TxID:   hex.EncodeToString(tx.ID),
					Vout:   outIdx,
					Amount: out.Value,
				})
			}
		}
	}

	json.NewEncoder(w).Encode(response)
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

func (rs *RestServer) getTransactions(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	addr := vars["address"]

	if !ValidateAddress(addr) {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid address"})
		return
	}

	txs := rs.P2P.Blockchain.FindTransactions(addr)

	var jsonTxs []JSONTransactionResponse
	for _, tx := range txs {
		jsonTxs = append(jsonTxs, ToJSONResponse(&tx))
	}

	json.NewEncoder(w).Encode(jsonTxs)
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
	rs.P2P.MempoolMux.Lock()
	defer rs.P2P.MempoolMux.Unlock()

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
