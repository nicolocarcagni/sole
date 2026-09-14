package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/gorilla/mux"

	"github.com/nicolocarcagni/sole/pkg/core"
	"github.com/nicolocarcagni/sole/pkg/wallet"
	"github.com/nicolocarcagni/sole/pkg/consensus"
	"github.com/nicolocarcagni/sole/pkg/p2p"
)

type RestServer struct {
	P2P *p2p.Server
	MempoolHub *EventHub
	BlockHub *EventHub
}

func StartRestServer(server *p2p.Server, listenHost string, port int) {
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
	router.Handle("/rawtx/{id}", readMW(http.HandlerFunc(rs.getRawTx))).Methods("GET")
	router.Handle("/transactions/{address}", readMW(http.HandlerFunc(rs.getTransactions))).Methods("GET")
	router.Handle("/transaction/{id}", readMW(http.HandlerFunc(rs.getTransaction))).Methods("GET")
	router.Handle("/proof/{id}", readMW(http.HandlerFunc(rs.getMerkleProof))).Methods("GET")
	router.Handle("/network/peers", readMW(http.HandlerFunc(rs.getPeers))).Methods("GET")
	router.Handle("/consensus/validators", readMW(http.HandlerFunc(rs.getValidators))).Methods("GET")

	// Stricter limit for Sending Transactions
	router.Handle("/tx/send", writeMW(http.HandlerFunc(rs.sendTx))).Methods("POST")

	// WebSocket Endpoints (no rate limiting — long-lived connections)
	router.HandleFunc("/ws/mempool", func(w http.ResponseWriter, r *http.Request) {
		handleWs(rs.MempoolHub, w, r)
	})
	router.HandleFunc("/ws/blocks", func(w http.ResponseWriter, r *http.Request) {
		handleWs(rs.BlockHub, w, r)
	})

	addr := fmt.Sprintf("%s:%d", listenHost, port)
	fmt.Printf("🚀 API p2p.Server started on http://%s\n", addr)

	srv := &http.Server{
		Handler:      CORSMiddleware(router),
		Addr:         addr,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}

func commonMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

type BalanceResponse struct {
	Address string `json:"address"`
	Balance int64  `json:"balance"`
}

type TipResponse struct {
	Height int    `json:"height"`
	Hash   string `json:"hash"`
}

type TxSendRequest struct {
	Hex  string  `json:"hex"`
	Fee  float64 `json:"fee"`
	Memo string  `json:"memo"`
}

type SuccessResponse struct {
	Status string `json:"status"`
	TxID   string `json:"txid,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type MerkleProofResponse struct {
	TxID        string       `json:"txid"`
	BlockHash   string       `json:"block_hash"`
	BlockHeight int          `json:"block_height"`
	MerkleRoot  string       `json:"merkle_root"`
	Proof       []core.MerkleStep `json:"proof"`
}

type RawTxResponse struct {
	Hex string `json:"hex"`
}

func (rs *RestServer) getMerkleProof(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	txIDHex := vars["id"]

	txID, err := hex.DecodeString(txIDHex)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid transaction ID format"})
		return
	}

	// Verify the transaction exists
	_, err = rs.P2P.Blockchain.FindTransaction(txID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "core.Transaction not found"})
		return
	}

	// Look up the block using the O(1) BadgerDB index
	var blockHash []byte
	err = rs.P2P.Blockchain.Database.View(func(txn *badger.Txn) error {
		item, badgerErr := txn.Get(append([]byte("tx-"), txID...))
		if badgerErr != nil {
			return badgerErr
		}
		blockHash, badgerErr = item.ValueCopy(nil)
		return badgerErr
	})

	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "core.Block containing the transaction not found"})
		return
	}

	block, err := rs.P2P.Blockchain.GetBlock(blockHash)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to retrieve block data"})
		return
	}

	var txHashes [][]byte
	for _, tx := range block.Transactions {
		txHashes = append(txHashes, tx.ID)
	}

	mTree := core.NewMerkleTree(txHashes)
	proof, err := mTree.GetMerklePath(txID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}

	response := MerkleProofResponse{
		TxID:        txIDHex,
		BlockHash:   hex.EncodeToString(block.Hash),
		BlockHeight: block.Height,
		MerkleRoot:  hex.EncodeToString(mTree.RootNode.Data),
		Proof:       proof,
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

type JSONTransactionResponse struct {
	ID        string       `json:"id"`
	Inputs    []JSONInput  `json:"inputs"`
	Outputs   []JSONOutput `json:"outputs"`
	Timestamp int64        `json:"timestamp"`
	Memo      string       `json:"memo,omitempty"`
}

type JSONInput struct {
	SenderAddress string `json:"sender_address"`
	Signature     string `json:"signature"`
}

type JSONOutput struct {
	ReceiverAddress string  `json:"receiver_address"`
	Value           int64   `json:"value"`
	ValueSole       float64 `json:"value_sole"`
}

type PeerResponse struct {
	TotalPeers int      `json:"total_peers"`
	Peers      []string `json:"peers"`
}

type ValidatorResponse struct {
	TotalValidators int      `json:"total_validators"`
	Validators      []string `json:"validators"`
}

func ToJSONResponse(tx *core.Transaction) JSONTransactionResponse {
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
				SenderAddress: core.AddressFromPubKeyHash(core.HashPubKey(vin.PubKey)),
				Signature:     hex.EncodeToString(vin.Signature),
			})
		}
	}

	// Outputs
	for _, vout := range tx.Vout {
		var receiverAddr string
		if vout.IsOPReturn() {
			receiverAddr = "OP_RETURN: " + string(vout.PubKeyHash)
		} else {
			receiverAddr = core.AddressFromPubKeyHash(vout.PubKeyHash)
		}

		outputs = append(outputs, JSONOutput{
			ReceiverAddress: receiverAddr,
			Value:           vout.Value,
			ValueSole:       float64(vout.Value) / 100000000.0,
		})
	}

	// Extract memo from OP_RETURN outputs
	var memo string
	for _, vout := range tx.Vout {
		if vout.IsOPReturn() {
			memo = string(vout.PubKeyHash)
			break
		}
	}

	return JSONTransactionResponse{
		ID:        hex.EncodeToString(tx.ID),
		Inputs:    inputs,
		Outputs:   outputs,
		Timestamp: tx.Timestamp,
		Memo:      memo,
	}
}

type JSONBlock struct {
	Timestamp     int64                     `json:"timestamp"`
	Height        int                       `json:"height"`
	PrevBlockHash string                    `json:"prev_block_hash"`
	Hash          string                    `json:"hash"`
	Transactions  []JSONTransactionResponse `json:"transactions"`
	Validator     string                    `json:"validator"`
	Signature     string                    `json:"signature"`
}

func ToJSONBlock(block *core.Block) JSONBlock {
	var jsonTxs []JSONTransactionResponse
	for _, tx := range block.Transactions {
		jsonTxs = append(jsonTxs, ToJSONResponse(tx))
	}

	return JSONBlock{
		Timestamp:     block.Timestamp,
		Height:        block.Height,
		PrevBlockHash: hex.EncodeToString(block.PrevBlockHash),
		Hash:          hex.EncodeToString(block.Hash),
		Transactions:  jsonTxs,
		Validator:     hex.EncodeToString(block.Validator),
		Signature:     hex.EncodeToString(block.Signature),
	}
}

func (rs *RestServer) getBalance(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	addr := vars["address"]

	if !wallet.ValidateAddress(addr) {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid address"})
		return
	}

	pubKeyHash, err := core.ExtractPubKeyHash(addr)
	if err != nil {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid address encoding"})
		return
	}

	utxos, err := rs.P2P.UTXOSet.FindUnspentOutputs(pubKeyHash)
	if err != nil {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to find unspent outputs"})
		return
	}
	balance := int64(0)

	for _, out := range utxos {
		balance += out.Value
	}

	json.NewEncoder(w).Encode(BalanceResponse{Address: addr, Balance: balance})
}

type UTXOResponse struct {
	TxID   string `json:"txid"`
	Vout   int    `json:"vout"`
	Amount int64  `json:"amount"`
}

func (rs *RestServer) getRawTx(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	txIDHex := vars["id"]

	txID, err := hex.DecodeString(txIDHex)
	if err != nil {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid transaction ID format"})
		return
	}

	tx, err := rs.P2P.Blockchain.FindTransaction(txID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "core.Transaction not found"})
		return
	}

	json.NewEncoder(w).Encode(RawTxResponse{Hex: hex.EncodeToString(tx.Serialize())})
}

func (rs *RestServer) getUTXOs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	addr := vars["address"]

	if !wallet.ValidateAddress(addr) {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid address"})
		return
	}

	pubKeyHash, err := core.ExtractPubKeyHash(addr)
	if err != nil {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid address encoding"})
		return
	}

	// 1. Identify Mempool Spends
	mempoolSpends := make(map[string]bool)
	rs.P2P.MempoolMux.Lock()
	for _, item := range rs.P2P.Mempool {
		for _, vin := range item.Tx.Vin {
			key := fmt.Sprintf("%x-%d", vin.Txid, vin.Vout)
			mempoolSpends[key] = true
		}
	}
	rs.P2P.MempoolMux.Unlock()

	utxos, err := rs.P2P.UTXOSet.FindAllUTXOs(pubKeyHash)
	if err != nil {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to find all UTXOs"})
		return
	}
	var response []UTXOResponse

	for _, u := range utxos {
		// 2. Filter out Mempool-locked UTXOs
		key := fmt.Sprintf("%s-%d", u.TxID, u.Vout)
		if mempoolSpends[key] {
			continue
		}

		response = append(response, UTXOResponse{
			TxID:   u.TxID,
			Vout:   u.Vout,
			Amount: u.Output.Value,
		})
	}

	if response == nil {
		response = make([]UTXOResponse, 0)
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
		json.NewEncoder(w).Encode(ErrorResponse{Error: "core.Block not found"})
		return
	}

	// Convert to JSONBlock to have enriched transaction data
	jsonBlock := ToJSONBlock(&block)
	json.NewEncoder(w).Encode(jsonBlock)
}

func (rs *RestServer) getTransactions(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	addr := vars["address"]

	if !wallet.ValidateAddress(addr) {
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

func (rs *RestServer) getTransaction(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	txIDHex := vars["id"]

	txID, err := hex.DecodeString(txIDHex)
	if err != nil {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid transaction ID format"})
		return
	}

	tx, err := rs.P2P.Blockchain.FindTransaction(txID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "core.Transaction not found"})
		return
	}

	jsonTx := ToJSONResponse(&tx)
	json.NewEncoder(w).Encode(jsonTx)
}

func (rs *RestServer) getPeers(w http.ResponseWriter, r *http.Request) {
	peers := rs.P2P.Host.Network().Peers()
	var peerList []string
	for _, p := range peers {
		peerList = append(peerList, p.String())
	}

	response := PeerResponse{
		TotalPeers: len(peerList),
		Peers:      peerList,
	}
	json.NewEncoder(w).Encode(response)
}

func (rs *RestServer) getValidators(w http.ResponseWriter, r *http.Request) {
	validators := consensus.AuthorizedValidators
	response := ValidatorResponse{
		TotalValidators: len(validators),
		Validators:      validators,
	}
	json.NewEncoder(w).Encode(response)
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
	tx := core.DeserializeTransaction(txBytes)

	// ── 1. Structural & Sanity Validations ──────────────────────────────────
	if len(tx.Vin) == 0 || len(tx.Vout) == 0 {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "core.Transaction has no inputs or outputs"})
		return
	}
	if tx.IsCoinbase() {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Coinbase transactions cannot be submitted via API"})
		return
	}
	if len(tx.ID) == 0 || !bytes.Equal(tx.ID, tx.Hash()) {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "core.Transaction ID integrity check failed"})
		return
	}

	// ── 2. Mempool Capacity ──────────────────────────────────────────────────
	rs.P2P.MempoolMux.Lock()
	mempoolLen := len(rs.P2P.Mempool)
	rs.P2P.MempoolMux.Unlock()
	if mempoolLen >= p2p.MaxMempoolSize {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Mempool is full, try again later"})
		return
	}

	// ── 3. Snapshot mempool for chained-transaction validation ──────────────
	rs.P2P.MempoolMux.Lock()
	mempoolSnapshot := make(map[string]core.MempoolItem, len(rs.P2P.Mempool))
	for k, v := range rs.P2P.Mempool {
		mempoolSnapshot[k] = v
	}
	rs.P2P.MempoolMux.Unlock()

	// ── 4. Input Validity & Fee Calculation ─────────────────────────────────
	// Uses the strict CalculateFee which rejects historically-spent outputs.
	fee, err := rs.P2P.UTXOSet.CalculateFee(&tx, mempoolSnapshot)
	if err != nil {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid inputs: " + err.Error()})
		return
	}
	if fee < 0 {
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Negative fee (%d)", fee)})
		return
	}

	// ── 5. Cryptographic Signature Verification ──────────────────────────────
	if rs.P2P.Blockchain.VerifyTransactionWithMempool(&tx, mempoolSnapshot) == false {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "core.Transaction invalid"})
		return
	}

	txID := hex.EncodeToString(tx.ID)

	// ── 6. Add to Mempool (with double-spend check) ──────────────────────────
	rs.P2P.MempoolMux.Lock()
	defer rs.P2P.MempoolMux.Unlock()

	if rs.P2P.Mempool[txID].Tx.ID == nil {
		// Check for mempool double-spend
		for _, vin := range tx.Vin {
			inputKey := hex.EncodeToString(vin.Txid) + ":" + fmt.Sprintf("%d", vin.Vout)
			for existingID, existing := range rs.P2P.Mempool {
				for _, evin := range existing.Tx.Vin {
					existingKey := hex.EncodeToString(evin.Txid) + ":" + fmt.Sprintf("%d", evin.Vout)
					if inputKey == existingKey {
						json.NewEncoder(w).Encode(ErrorResponse{
							Error: fmt.Sprintf("Double-spend: input %s already used by mempool TX %s", inputKey, existingID),
						})
						return
					}
				}
			}
		}

		rs.P2P.Mempool[txID] = core.MempoolItem{Tx: tx, AddedAt: time.Now().Unix()}
		fmt.Printf("API: core.Transaction added to Mempool: %s (Fee: %d)\n", txID, fee)
		BroadcastMempoolTx(rs.MempoolHub, &tx)

		// Broadcast Inv
		peers := rs.P2P.Host.Network().Peers()
		for _, p := range peers {
			rs.P2P.SendInv(p, "tx", [][]byte{tx.ID})
		}

		json.NewEncoder(w).Encode(SuccessResponse{Status: "success", TxID: txID})
	} else {
		json.NewEncoder(w).Encode(ErrorResponse{Error: "core.Transaction already in mempool or exists"})
	}
}
