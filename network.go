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
	"os"
	"sort"
	"strings"
	"time"

	"sync"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/multiformats/go-multiaddr"
)

const (
	protocolID         = "/sole/1.0.0"
	discoveryNamespace = "sole_p2p"
)

var (
	commandLength    = 12
	DefaultBootnodes = []string{
		"/dns4/sole.nicolocarcagni.dev/tcp/3000/p2p/12D3KooWEtsfPSAJjJMueguEWXkK35PmyBSyiUvKCGsAEHPGXFSG",
	}
)

// Server represents the P2P server
type Server struct {
	Host             host.Host
	Blockchain       *Blockchain
	UTXOSet          *UTXOSet
	MinerAddr        string
	ValidatorPrivKey *ecdsa.PrivateKey
	KnownPeers       map[string]string // PeerID string -> Addr
	Mempool          map[string]Transaction
	MempoolMux       sync.Mutex

	// IBD (Initial Block Download) state
	SyncingFrom    peer.ID        // Peer we are currently syncing from
	IsSyncing      bool           // True while IBD is in progress
	BlockBuffer    map[int]*Block // Height → Block buffer for ordered application
	ExpectedBlocks int            // Total blocks expected during IBD
	BlockBufferMux sync.Mutex
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
		// Trigger Handshake immediately upon connection (only if running as Server)
		if n.server != nil {
			n.server.SendVersion(pi.ID)
		}
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
	if len(id) > 12 {
		return id[:6] + "..." + id[len(id)-6:]
	}
	return id
}

// ServerConfig holds P2P configuration
type ServerConfig struct {
	ListenHost string
	Port       int
	PublicIP   string
	PublicDNS  string
	Bootnodes  []string
	MinerAddr  string
	PrivKey    *ecdsa.PrivateKey
	NodeKey    crypto.PrivKey // Identity Key
}

// LoadOrGenerateNodeKey manages persistent P2P identity
func LoadOrGenerateNodeKey(keyFile string) (crypto.PrivKey, error) {
	// Check if file exists
	if _, err := os.Stat(keyFile); err == nil {
		// LOAD
		data, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, err
		}
		return crypto.UnmarshalPrivateKey(data)
	}

	// GENERATE
	fmt.Println("🔑 Generating new P2P Identity Key...")
	priv, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		return nil, err
	}

	// SAVE
	data, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	err = os.WriteFile(keyFile, data, 0600)
	return priv, err
}

// NewServer initializes the P2P server
// NewServer initializes the P2P server
func NewServer(cfg ServerConfig) *Server {
	// Use persistent identity
	priv := cfg.NodeKey

	listenAddr := fmt.Sprintf("/ip4/%s/tcp/%d", cfg.ListenHost, cfg.Port)
	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(listenAddr),
		libp2p.Identity(priv),
		// Enable NAT traversal
		// libp2p.NATPortMap(), // Optional but good practice
	}

	// Handle Public IP/DNS Announcement (NAT Traversal)
	if cfg.PublicDNS != "" {
		externalAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/dns4/%s/tcp/%d", cfg.PublicDNS, cfg.Port))
		if err != nil {
			log.Fatalf("Fatal: Invalid Public DNS Multiaddr: %v", err)
		}
		addrFactory := func(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
			return []multiaddr.Multiaddr{externalAddr}
		}
		opts = append(opts, libp2p.AddrsFactory(addrFactory))
		opts = append(opts, libp2p.ForceReachabilityPublic())
	} else if cfg.PublicIP != "" {
		externalAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", cfg.PublicIP, cfg.Port))
		if err != nil {
			log.Fatalf("Fatal: Invalid Public IP Multiaddr: %v", err)
		}

		// Factory to force announcing ONLY the external address
		addrFactory := func(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
			return []multiaddr.Multiaddr{externalAddr}
		}
		opts = append(opts, libp2p.AddrsFactory(addrFactory))
		opts = append(opts, libp2p.ForceReachabilityPublic())
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		log.Fatalf("Fatal: Failed to start libp2p host: %v", err)
	}

	// Using Default Bootnodes if needed
	bootnodesToUse := cfg.Bootnodes
	if len(bootnodesToUse) == 0 {
		bootnodesToUse = DefaultBootnodes
	}

	chain := ContinueBlockchain("")
	UTXOSet := &UTXOSet{chain}

	server := &Server{
		Host:             h,
		Blockchain:       chain,
		UTXOSet:          UTXOSet,
		MinerAddr:        cfg.MinerAddr,
		ValidatorPrivKey: cfg.PrivKey,
		KnownPeers:       make(map[string]string),
		Mempool:          make(map[string]Transaction),
		BlockBuffer:      make(map[int]*Block),
	}

	// Set Stream Handler
	h.SetStreamHandler(protocolID, server.HandleStream)

	// Setup mDNS Discovery (Still useful for LAN)
	notifee := &discoveryNotifee{h: h, server: server}
	ser := mdns.NewMdnsService(h, discoveryNamespace, notifee)
	if err := ser.Start(); err != nil {
		log.Panic(err)
	}

	// Bootstrap (Internet Discovery)
	if len(bootnodesToUse) > 0 {
		go server.Bootstrap(bootnodesToUse)
	}

	fmt.Println()
	fmt.Println(ColorGreen + "──────────────────────────────────────────────────────────────────────" + ColorReset)
	fmt.Printf(" ☀️  SOLE NODE STARTED (Port: "+ColorYellow+"%d"+ColorReset+")\n", cfg.Port)
	fmt.Printf(" 🆔 Peer ID: "+ColorCyan+"%s"+ColorReset+"\n", h.ID().String())
	fmt.Println(ColorGreen + "──────────────────────────────────────────────────────────────────────" + ColorReset)
	fmt.Println()
	fmt.Println(" 🔗 Listen Addresses:")

	for _, addr := range h.Addrs() {
		// Construct full multiaddr: /ip4/x.x.x.x/tcp/3000/p2p/Qm...
		fullAddr := fmt.Sprintf("%s/p2p/%s", addr, h.ID().String())

		// Visual emphasis for public/LAN IPs
		if strings.Contains(fullAddr, "/127.0.0.1/") {
			fmt.Printf("   "+ColorYellow+"(Local)"+ColorReset+"  %s\n", fullAddr)
		} else {
			fmt.Printf("   "+ColorGreen+"👉(Public)"+ColorReset+" %s\n", fullAddr)
		}
	}
	return server
}

// Bootstrap attempts to connect to seed nodes
func (s *Server) Bootstrap(bootnodes []string) {
	fmt.Printf("🔄 Bootstrapping: Connecting to %d seed nodes...\n", len(bootnodes))

	validNodes := 0
	for _, addr := range bootnodes {
		ma, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			fmt.Printf("⚠️  Invalid bootnode address %s: %s\n", addr, err)
			continue
		}

		pi, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			fmt.Printf("⚠️  Invalid bootnode info %s: %s\n", addr, err)
			continue
		}

		// Self-Dial Check
		if pi.ID == s.Host.ID() {
			fmt.Println("ℹ️  Skipping bootnode (Self-Dial detected)")
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = s.Host.Connect(ctx, *pi)
		cancel()

		if err != nil {
			fmt.Printf("⚠️  Failed to connect to bootnode %s: %s\n", ShortID(pi.ID.String()), err)
		} else {
			fmt.Printf("✅ Connected to bootnode: %s\n", ShortID(pi.ID.String()))
			validNodes++
			// Trigger sync immediately
			s.SendVersion(pi.ID)
		}
	}

	if validNodes > 0 {
		fmt.Println("🚀 Bootstrap completed successfully.")
	} else if len(bootnodes) > 0 {
		fmt.Println("⚠️  Bootstrap failed: No bootnodes reachable.")
	}
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
		return
	}

	fmt.Printf("🤝 [Handshake] Connected to: %s (Remote) | Version: %d | BestHeight: %d\n", ShortID(peerID.String()), payload.Version, payload.BestHeight)
	s.KnownPeers[peerID.String()] = payload.AddrFrom

	myBestHeight := s.Blockchain.GetBestHeight()
	foreignerBestHeight := payload.BestHeight

	if myBestHeight < foreignerBestHeight {
		// Initialize IBD state
		s.BlockBufferMux.Lock()
		s.IsSyncing = true
		s.SyncingFrom = peerID
		s.BlockBuffer = make(map[int]*Block)
		s.BlockBufferMux.Unlock()

		fmt.Printf("📦 [IBD] Starting sync from %s (local: %d, remote: %d)\n", ShortID(peerID.String()), myBestHeight, foreignerBestHeight)
		s.SendGetBlocks(peerID)
	} else if myBestHeight > foreignerBestHeight {
		s.SendVersion(peerID)
	}
}

func (s *Server) HandleInv(request []byte, peerID peer.ID) {
	var payload Inv
	dec := gob.NewDecoder(bytes.NewReader(request))
	dec.Decode(&payload)

	if payload.Type == "block" {
		// Filter out blocks we already have
		var needed [][]byte
		for _, blockHash := range payload.Items {
			_, err := s.Blockchain.GetBlock(blockHash)
			if err != nil {
				// Block not found locally → we need it
				needed = append(needed, blockHash)
			}
		}

		if len(needed) > 0 {
			s.BlockBufferMux.Lock()
			s.ExpectedBlocks = len(needed)
			s.BlockBufferMux.Unlock()

			fmt.Printf("📦 [IBD] Requesting %d missing blocks from %s\n", len(needed), ShortID(peerID.String()))
			for _, b := range needed {
				s.SendGetData(peerID, "block", b)
			}
		} else {
			// All blocks already present, end IBD if active
			s.BlockBufferMux.Lock()
			if s.IsSyncing {
				s.IsSyncing = false
				fmt.Println("✅ [IBD] Already in sync.")
			}
			s.BlockBufferMux.Unlock()
		}
	}
	if payload.Type == "tx" {
		if len(payload.Items) > 0 {
			txID := payload.Items[0]
			if s.Mempool[hex.EncodeToString(txID)].ID == nil {
				s.SendGetData(peerID, "tx", txID)
			}
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
		fmt.Printf("📦 [P2P] Data Request (Block) | Hash: %x | Peer: %s\n", payload.ID[:4], ShortID(peerID.String()))
		block, err := s.Blockchain.GetBlock(payload.ID)
		if err != nil {
			fmt.Printf("⚠️  Object (Block) not found for Hash: %x\n", payload.ID)
			return
		}
		s.SendBlock(peerID, &block)
	}

	if payload.Type == "tx" {
		txID := hex.EncodeToString(payload.ID)
		fmt.Printf("📦 [P2P] Data Request (Tx) | Hash: %s... | Peer: %s\n", txID[:8], ShortID(peerID.String()))
		tx, ok := s.Mempool[txID]
		if !ok {
			fmt.Printf("⚠️  Object (Tx) not found in Mempool: %s\n", txID)
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

	s.BlockBufferMux.Lock()
	isSyncing := s.IsSyncing
	s.BlockBufferMux.Unlock()

	if isSyncing {
		// === IBD MODE: Buffer blocks and apply in order ===
		s.BlockBufferMux.Lock()
		s.BlockBuffer[block.Height] = block
		buffered := len(s.BlockBuffer)
		expected := s.ExpectedBlocks
		s.BlockBufferMux.Unlock()

		fmt.Printf("📦 [IBD] Buffered block %d (hash: %x) [%d/%d]\n", block.Height, block.Hash[:4], buffered, expected)

		// Check if we have all expected blocks
		if buffered >= expected && expected > 0 {
			s.applyBufferedBlocks()
		}
	} else {
		// === NORMAL MODE: Apply single block immediately ===
		fmt.Printf("Received new block! Hash: %x Height: %d\n", block.Hash, block.Height)

		// Validate UTXOs (Double-spend check) before processing the block
		if !s.UTXOSet.ValidateBlockTransactions(block) {
			fmt.Printf("⛔ Block %x rejected: Contains double-spends or invalid inputs.\n", block.Hash)
			return
		}

		if s.Blockchain.AddBlock(block) {
			s.UTXOSet.Update(block)
			fmt.Printf("✅ Block added %x and UTXO set updated.\n", block.Hash)
		} else {
			fmt.Printf("Block discarded or duplicate: %x\n", block.Hash)
		}

		// Clean mempool
		if len(s.Mempool) > 0 {
			for _, tx := range block.Transactions {
				txID := hex.EncodeToString(tx.ID)
				delete(s.Mempool, txID)
			}
		}
	}
}

// applyBufferedBlocks sorts buffered blocks by height and applies them chronologically,
// then performs a single UTXO reindex.
func (s *Server) applyBufferedBlocks() {
	s.BlockBufferMux.Lock()
	defer func() {
		// Reset IBD state
		s.IsSyncing = false
		s.BlockBuffer = make(map[int]*Block)
		s.ExpectedBlocks = 0
		s.BlockBufferMux.Unlock()
	}()

	// Collect and sort heights
	heights := make([]int, 0, len(s.BlockBuffer))
	for h := range s.BlockBuffer {
		heights = append(heights, h)
	}
	sort.Ints(heights)

	fmt.Printf("🔄 [IBD] Applying %d blocks in chronological order (height %d → %d)...\n",
		len(heights), heights[0], heights[len(heights)-1])

	applied := 0
	for _, h := range heights {
		block := s.BlockBuffer[h]
		if s.Blockchain.AddBlock(block) {
			applied++
		}
	}

	fmt.Printf("✅ [IBD] Sync complete. Applied %d blocks.\n", applied)

	// Full UTXO reindex from the now-complete chain
	fmt.Println("🔄 [IBD] Rebuilding UTXO set (Reindex)...")
	s.UTXOSet.Reindex()
	fmt.Println("✅ [IBD] UTXO Reindex complete.")
}

func (s *Server) HandleTx(request []byte, peerID peer.ID) {
	var payload TxMsg
	dec := gob.NewDecoder(bytes.NewReader(request))
	dec.Decode(&payload)

	txData := payload.Transaction
	tx := DeserializeTransaction(txData)

	s.MempoolMux.Lock()
	defer s.MempoolMux.Unlock()

	if s.Mempool[hex.EncodeToString(tx.ID)].ID == nil {
		fmt.Printf("New Transaction in Mempool: %x\n", tx.ID)
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
	// s.AttemptMine() // Removed for Periodic Mining
}

// StartMiningLoop periodically checks mempool to mine new blocks
func (s *Server) StartMiningLoop() {
	if s.MinerAddr == "" {
		return
	}
	fmt.Println("⛏️  Mining Loop started (Interval: 10s)")
	ticker := time.NewTicker(10 * time.Second)

	for range ticker.C {
		s.AttemptMine()
	}
}

// AttemptMine tries to mine a block if conditions are met
func (s *Server) AttemptMine() {
	if s.MinerAddr == "" || s.ValidatorPrivKey == nil {
		return
	}

	s.MempoolMux.Lock()
	defer s.MempoolMux.Unlock()

	if len(s.Mempool) == 0 {
		return
	}

	fmt.Println("Forging new block with mempool transactions...")
	var txs []*Transaction
	for id := range s.Mempool {
		tx := s.Mempool[id]
		if s.Blockchain.VerifyTransaction(&tx) {
			txs = append(txs, &tx)
		} else {
			delete(s.Mempool, id) // Clear invalid tx
		}
	}

	if len(txs) == 0 {
		fmt.Println("All transactions in mempool are invalid.")
		return
	}

	// Calculate Dynamic Block Subsidy (Tokenomics)
	bestHeight := s.Blockchain.GetBestHeight()
	nextHeight := bestHeight + 1
	subsidy := s.Blockchain.GetBlockSubsidy(nextHeight)

	// Add Coinbase for Miner
	cbTx := NewCoinbaseTX(s.MinerAddr, "", subsidy) // Dynamic Reward

	// [SECURITY FIX] Ensure the set of transactions doesn't contain double-spends
	prospectiveBlock := &Block{Transactions: append([]*Transaction{cbTx}, txs...)}
	if !s.UTXOSet.ValidateBlockTransactions(prospectiveBlock) {
		fmt.Println("⚠️  Mempool contains conflicting transactions. Clearing Mempool.")
		s.Mempool = make(map[string]Transaction)
		return
	}

	txs = append([]*Transaction{cbTx}, txs...) // Coinbase first

	newBlock := s.Blockchain.ForgeBlock(txs, *s.ValidatorPrivKey)
	s.UTXOSet.Update(newBlock)

	// Clear Mempool
	s.Mempool = make(map[string]Transaction)

	fmt.Printf("New block forged: %x (UTXO updated)\n", newBlock.Hash)

	// Broadcast new block
	peers := s.Host.Network().Peers()
	for _, p := range peers {
		s.SendInv(p, "block", [][]byte{newBlock.Hash})
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
