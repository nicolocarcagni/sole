package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"log"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

const (
	protocolID         = "/sole/1.0.0"
	discoveryNamespace = "sole_p2p"
)

var (
	commandLength = 12
)

// Server represents the P2P server
type Server struct {
	Host             host.Host
	Blockchain       *Blockchain
	MinerAddr        string
	ValidatorPrivKey *ecdsa.PrivateKey
	KnownPeers       map[string]string // PeerID string -> Addr
	Mempool          map[string]Transaction
}

type discoveryNotifee struct {
	h      host.Host
	server *Server
}

func (n *discoveryNotifee) HandlePeerFound(pi peer.AddrInfo) {
	// 1. Filter Self-Address (Avoid Self-Dialing)
	if pi.ID == n.h.ID() {
		// fmt.Printf("DEBUG: Found self %s, skipping.\n", ShortID(pi.ID.String()))
		return
	}

	// fmt.Printf("Peer discovered: %s\n", ShortID(pi.ID.String()))

	err := n.h.Connect(context.Background(), pi)
	if err != nil {
		errMsg := err.Error()
		// 2. Improve Error Handling
		if errMsg == "dial to self attempted" {
			// Ignore, expected behavior
			return
		} else if contains(errMsg, "i/o timeout") || contains(errMsg, "no good addresses") {
			// Debug level for network noise
			// fmt.Printf("DEBUG: Connect timeout %s\n", ShortID(pi.ID.String()))
		} else if contains(errMsg, "unexpected handshake message") || contains(errMsg, "tls") {
			fmt.Printf("⚠️  [P2P] TLS Error connecting to %s: %s\n", ShortID(pi.ID.String()), err)
		} else {
			fmt.Printf("⚠️  [P2P] Error connecting to %s: %s\n", ShortID(pi.ID.String()), err)
		}
	} else {
		// Trigger Handshake immediately upon connection
		// fmt.Printf("🔌 Connected to %s, sending Version...\n", ShortID(pi.ID.String()))
		n.server.SendVersion(pi.ID)
	}
}

// Helper to check substring
func contains(s, substr string) bool {
	// Simple string check
	// We need "strings" package if we use strings.Contains
	// But since we didn't import "strings", let's use the buffer way or just add import.
	// Actually, "strings" is standard. I should add it to imports.
	// But to save steps, I implemented it with bytes above.
	// Wait, in previous step I used bytes.Contains([]byte(s), ...).
	// bytes package IS imported.
	return bytes.Contains([]byte(s), []byte(substr))
}

// ShortID returns the first 6 characters of a PeerID
func ShortID(id string) string {
	if len(id) > 6 {
		return id[:6] + "..."
	}
	return id
}

// NewServer initializes the P2P server
func NewServer(port int, minerAddress string, validatorPrivKey *ecdsa.PrivateKey) *Server {
	// Create LibP2P Host
	priv, _, _ := crypto.GenerateKeyPair(crypto.Secp256k1, 256)

	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)),
		libp2p.Identity(priv),
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		log.Panic(err)
	}

	chain := ContinueBlockchain("")

	server := &Server{
		Host:             h,
		Blockchain:       chain,
		MinerAddr:        minerAddress,
		ValidatorPrivKey: validatorPrivKey,
		KnownPeers:       make(map[string]string),
		Mempool:          make(map[string]Transaction),
	}

	// Set Stream Handler
	h.SetStreamHandler(protocolID, server.HandleStream)

	// Setup mDNS Discovery
	notifee := &discoveryNotifee{h: h, server: server}
	ser := mdns.NewMdnsService(h, discoveryNamespace, notifee)
	if err := ser.Start(); err != nil {
		log.Panic(err)
	}

	fmt.Printf("Server listening on %s with peer ID %s\n", h.Addrs()[0], ShortID(h.ID().String()))
	return server
}

// Start runs the P2P server loop (blocking)
func (s *Server) Start() {
	fmt.Println("Waiting for connections...")
	// Loop removed to avoid spam. Handshake is now event-driven in HandlePeerFound.
	select {} // block forever
}

func (s *Server) HandleStream(stream network.Stream) {
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
	go s.ReadData(rw, stream.Conn().RemotePeer())
}

func (s *Server) ReadData(rw *bufio.ReadWriter, peerID peer.ID) {
	// Read all data until EOF (stream closed)
	payload, err := io.ReadAll(rw)
	if err != nil {
		fmt.Println("Error reading stream:", err)
		return
	}

	if len(payload) < commandLength {
		return
	}

	command := BytesToCommand(payload[:commandLength])
	content := payload[commandLength:]

	// fmt.Printf("Received %s command from %s\n", command, peerID.String())

	switch command {
	case "version":
		s.HandleVersion(content, peerID)
	case "inv":
		s.HandleInv(content, peerID)
	case "getblocks":
		s.HandleGetBlocks(content, peerID)
	case "getdata":
		s.HandleGetData(content, peerID)
	case "block":
		s.HandleBlock(content, peerID)
	case "tx":
		s.HandleTx(content, peerID)
	default:
		fmt.Println("Unknown command")
	}
}

// Helper structs for messages
type Version struct {
	Version    int
	BestHeight int
	AddrFrom   string
}

type Inv struct {
	AddrFrom string
	Type     string
	Items    [][]byte
}

type GetData struct {
	AddrFrom string
	Type     string
	ID       []byte
}

type BlockMsg struct {
	AddrFrom string
	Block    []byte
}

type TxMsg struct {
	AddrFrom    string
	Transaction []byte
}

// Handlers

func (s *Server) HandleVersion(request []byte, peerID peer.ID) {
	var payload Version
	dec := gob.NewDecoder(bytes.NewReader(request))
	dec.Decode(&payload)

	// Duplicate Handshake Check
	if _, ok := s.KnownPeers[peerID.String()]; ok {
		// fmt.Printf("DEBUG: Ignored redundant Version from %s\n", ShortID(peerID.String()))
		return
	}

	fmt.Printf("🤝 [P2P] Handshake (Version) | BestHeight: %d | Peer: %s\n", payload.BestHeight, ShortID(peerID.String()))
	s.KnownPeers[peerID.String()] = payload.AddrFrom

	myBestHeight := s.Blockchain.GetBestHeight()
	foreignerBestHeight := payload.BestHeight

	if myBestHeight < foreignerBestHeight {
		s.SendGetBlocks(peerID)
	} else if myBestHeight > foreignerBestHeight {
		s.SendVersion(peerID)
	}
}

func (s *Server) HandleInv(request []byte, peerID peer.ID) {
	var payload Inv
	dec := gob.NewDecoder(bytes.NewReader(request))
	dec.Decode(&payload)

	// fmt.Printf("Received inventory with %d %s\n", len(payload.Items), payload.Type)

	if payload.Type == "block" {
		blocksInTransit := payload.Items
		for _, b := range blocksInTransit {
			s.SendGetData(peerID, "block", b)
		}
	}
	if payload.Type == "tx" {
		txID := payload.Items[0]
		if s.Mempool[hex.EncodeToString(txID)].ID == nil {
			s.SendGetData(peerID, "tx", txID)
		}
	}
}

func (s *Server) HandleGetBlocks(request []byte, peerID peer.ID) {
	hashes := s.Blockchain.GetBlockHashes()
	s.SendInv(peerID, "block", hashes)
}

func (s *Server) HandleGetData(request []byte, peerID peer.ID) {
	var payload GetData
	dec := gob.NewDecoder(bytes.NewReader(request))
	dec.Decode(&payload)

	if payload.Type == "block" {
		fmt.Printf("📦 [P2P] Richiesta Dati (Block) | Hash: %x | Peer: %s\n", payload.ID[:4], ShortID(peerID.String()))
		block, err := s.Blockchain.GetBlock(payload.ID)
		if err != nil {
			fmt.Printf("⚠️  Oggetto (Block) non trovato per Hash: %x\n", payload.ID)
			return
		}
		s.SendBlock(peerID, &block)
	}

	if payload.Type == "tx" {
		txID := hex.EncodeToString(payload.ID)
		fmt.Printf("📦 [P2P] Richiesta Dati (Tx) | Hash: %s... | Peer: %s\n", txID[:8], ShortID(peerID.String()))
		tx, ok := s.Mempool[txID]
		if !ok {
			fmt.Printf("⚠️  Oggetto (Tx) non trovato in Mempool: %s\n", txID)
			return
		}
		s.SendTx(peerID, &tx)
	}
}

func (s *Server) HandleBlock(request []byte, peerID peer.ID) {
	var payload BlockMsg
	dec := gob.NewDecoder(bytes.NewReader(request))
	dec.Decode(&payload)

	block := DeserializeBlock(payload.Block)
	fmt.Printf("ricevuto nuovo blocco! Hash: %x\n", block.Hash)

	s.Blockchain.AddBlock(block)
	fmt.Printf("Blocco aggiunto %x\n", block.Hash)

	if len(s.Mempool) > 0 {
		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)
			delete(s.Mempool, txID)
		}
	}
}

func (s *Server) HandleTx(request []byte, peerID peer.ID) {
	var payload TxMsg
	dec := gob.NewDecoder(bytes.NewReader(request))
	dec.Decode(&payload)

	txData := payload.Transaction
	tx := DeserializeTransaction(txData)

	if s.Mempool[hex.EncodeToString(tx.ID)].ID == nil {
		fmt.Printf("Nuova Transazione in Mempool: %x\n", tx.ID)
		s.Mempool[hex.EncodeToString(tx.ID)] = tx

		// Propagate
		peers := s.Host.Network().Peers()
		for _, p := range peers {
			if p != peerID {
				s.SendInv(p, "tx", [][]byte{tx.ID})
			}
		}
	} else {
		// fmt.Printf("Transazione %x già in mempool\n", tx.ID)
	}

	// Mine if Miner (and has valid privKey)
	if s.MinerAddr != "" && s.ValidatorPrivKey != nil && len(s.Mempool) >= 1 {
		fmt.Println("Forging nuovo blocco con transazioni della mempool...")
		var txs []*Transaction
		for id := range s.Mempool {
			tx := s.Mempool[id]
			if s.Blockchain.VerifyTransaction(&tx) {
				txs = append(txs, &tx)
			}
		}

		if len(txs) == 0 {
			fmt.Println("Tutte le transazioni in mempool sono invalide.")
			return
		}

		// Add Coinbase for Miner
		cbTx := NewCoinbaseTX(s.MinerAddr, "", 20) // Miner Reward
		txs = append([]*Transaction{cbTx}, txs...) // Coinbase first

		newBlock := s.Blockchain.ForgeBlock(txs, *s.ValidatorPrivKey)

		// Clear Mempool
		for _, tx := range txs {
			delete(s.Mempool, hex.EncodeToString(tx.ID))
		}

		fmt.Printf("Nuovo blocco forgiato: %x\n", newBlock.Hash)

		// Broadcast new block
		peers := s.Host.Network().Peers()
		for _, p := range peers {
			s.SendInv(p, "block", [][]byte{newBlock.Hash})
		}
	}
}

// Senders

func (s *Server) SendVersion(peerID peer.ID) {
	bestHeight := s.Blockchain.GetBestHeight()
	payload := GobEncode(Version{1, bestHeight, s.Host.ID().String()})
	request := append(CommandToBytes("version"), payload...)
	s.SendData(peerID, request)
}

func (s *Server) SendGetBlocks(peerID peer.ID) {
	payload := GobEncode(Version{1, 0, s.Host.ID().String()})
	request := append(CommandToBytes("getblocks"), payload...)
	s.SendData(peerID, request)
}

func (s *Server) SendInv(peerID peer.ID, kind string, items [][]byte) {
	inventory := Inv{s.Host.ID().String(), kind, items}
	payload := GobEncode(inventory)
	request := append(CommandToBytes("inv"), payload...)
	s.SendData(peerID, request)
}

func (s *Server) SendGetData(peerID peer.ID, kind string, id []byte) {
	payload := GobEncode(GetData{s.Host.ID().String(), kind, id})
	request := append(CommandToBytes("getdata"), payload...)
	s.SendData(peerID, request)
}

func (s *Server) SendBlock(peerID peer.ID, block *Block) {
	data := BlockMsg{s.Host.ID().String(), block.Serialize()}
	payload := GobEncode(data)
	request := append(CommandToBytes("block"), payload...)
	s.SendData(peerID, request)
}

func (s *Server) SendTx(peerID peer.ID, tx *Transaction) {
	data := TxMsg{s.Host.ID().String(), tx.Serialize()}
	payload := GobEncode(data)
	request := append(CommandToBytes("tx"), payload...)
	s.SendData(peerID, request)
}

func (s *Server) SendData(peerID peer.ID, data []byte) {
	stream, err := s.Host.NewStream(context.Background(), peerID, protocolID)
	if err != nil {
		return
	}
	defer stream.Close()

	_, err = stream.Write(data)
	if err != nil {
		// log.Panic(err)
	}
}

// Utils

func CommandToBytes(command string) []byte {
	var bytes [12]byte // commandLength
	for i, c := range command {
		bytes[i] = byte(c)
	}
	return bytes[:]
}

func BytesToCommand(bytes []byte) string {
	var command []byte
	for _, b := range bytes {
		if b != 0x0 {
			command = append(command, b)
		}
	}
	return string(command)
}

func GobEncode(data interface{}) []byte {
	var buff bytes.Buffer
	enc := gob.NewEncoder(&buff)
	err := enc.Encode(data)
	if err != nil {
		log.Panic(err)
	}
	return buff.Bytes()
}

func DeserializeTransaction(data []byte) Transaction {
	var tx Transaction
	decoder := gob.NewDecoder(bytes.NewReader(data))
	err := decoder.Decode(&tx)
	if err != nil {
		log.Panic(err)
	}
	return tx
}
