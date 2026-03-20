package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/binary"
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
	protocolID         = "/sole/3.0.0"
	discoveryNamespace = "sole_p2p"
)

var (
	commandLength    = 12
	DefaultBootnodes = []string{
		"/dns4/sole.nicolocarcagni.dev/tcp/3000/p2p/12D3KooWEtsfPSAJjJMueguEWXkK35PmyBSyiUvKCGsAEHPGXFSG",
	}
)

type MempoolItem struct {
	Tx      Transaction
	AddedAt int64
}

type Server struct {
	Host             host.Host
	Blockchain       *Blockchain
	UTXOSet          *UTXOSet
	MinerAddr        string
	ValidatorPrivKey *ecdsa.PrivateKey
	KnownPeers       map[string]string // PeerID string -> Addr
	KnownPeersMux    sync.RWMutex
	Mempool          map[string]MempoolItem
	MempoolMux       sync.Mutex

	MempoolHub *EventHub
	BlockHub   *EventHub

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

func contains(s, substr string) bool {
	// Simple string check using bytes package
	return bytes.Contains([]byte(s), []byte(substr))
}

func ShortID(id string) string {
	if len(id) > 12 {
		return id[:6] + "..." + id[len(id)-6:]
	}
	return id
}

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
func NewServer(cfg ServerConfig) *Server {
	// Use persistent identity
	priv := cfg.NodeKey

	listenAddr := fmt.Sprintf("/ip4/%s/tcp/%d", cfg.ListenHost, cfg.Port)
	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(listenAddr),
		libp2p.Identity(priv),
		// Enable NAT traversal
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

	mempoolHub := NewEventHub()
	go mempoolHub.Run()
	blockHub := NewEventHub()
	go blockHub.Run()

	server := &Server{
		Host:             h,
		Blockchain:       chain,
		UTXOSet:          UTXOSet,
		MinerAddr:        cfg.MinerAddr,
		ValidatorPrivKey: cfg.PrivKey,
		KnownPeers:       make(map[string]string),
		Mempool:          make(map[string]MempoolItem),
		MempoolHub:       mempoolHub,
		BlockHub:         blockHub,
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

	select {} // block forever
}

func (s *Server) HandleStream(stream network.Stream) {
	// Set a generous read deadline for large block transfers
	stream.SetReadDeadline(time.Now().Add(2 * time.Minute))
	go s.ReadData(stream, stream.Conn().RemotePeer())
}

func (s *Server) ReadData(stream network.Stream, peerID peer.ID) {
	defer stream.Close()

	// Read 4-byte length prefix (big-endian)
	lenBuf := make([]byte, 4)
	_, err := io.ReadFull(stream, lenBuf)
	if err != nil {
		if err != io.EOF {
			log.Printf("Error reading length prefix from %s: %v", ShortID(peerID.String()), err)
		}
		return
	}

	payloadLen := binary.BigEndian.Uint32(lenBuf)
	if payloadLen == 0 || payloadLen > 8*1024*1024 { // 8MB safety cap
		log.Printf("⚠️ Invalid payload length from %s: %d bytes. Dropping.", ShortID(peerID.String()), payloadLen)
		return
	}

	// Read exactly payloadLen bytes
	payload := make([]byte, payloadLen)
	_, err = io.ReadFull(stream, payload)
	if err != nil {
		log.Printf("Error reading payload (%d bytes) from %s: %v", payloadLen, ShortID(peerID.String()), err)
		return
	}

	if len(payload) < commandLength {
		return
	}

	command := BytesToCommand(payload[:commandLength])
	content := payload[commandLength:]

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

func (s *Server) HandleVersion(request []byte, peerID peer.ID) {
	var payload Version
	dec := gob.NewDecoder(bytes.NewReader(request))
	dec.Decode(&payload)

	// Duplicate Handshake Check
	s.KnownPeersMux.RLock()
	_, ok := s.KnownPeers[peerID.String()]
	s.KnownPeersMux.RUnlock()
	if ok {
		return
	}

	fmt.Printf("🤝 [Handshake] Connected to: %s (Remote) | Version: %d | BestHeight: %d\n", ShortID(peerID.String()), payload.Version, payload.BestHeight)
	
	s.KnownPeersMux.Lock()
	s.KnownPeers[peerID.String()] = payload.AddrFrom
	s.KnownPeersMux.Unlock()

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
		var needed [][]byte
		for _, blockHash := range payload.Items {
			_, err := s.Blockchain.GetBlock(blockHash)
			if err != nil {
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
			txID := hex.EncodeToString(payload.Items[0])
			s.MempoolMux.Lock()
			exists := s.Mempool[txID].Tx.ID != nil
			s.MempoolMux.Unlock()
			if !exists {
				s.SendGetData(peerID, "tx", payload.Items[0])
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
		s.MempoolMux.Lock()
		item, ok := s.Mempool[txID]
		s.MempoolMux.Unlock()
		if !ok {
			fmt.Printf("⚠️  Object (Tx) not found in Mempool: %s\n", txID)
			return
		}
		s.SendTx(peerID, &item.Tx)
	}
}

func (s *Server) HandleBlock(request []byte, peerID peer.ID) {
	var payload BlockMsg
	dec := gob.NewDecoder(bytes.NewReader(request))
	if err := dec.Decode(&payload); err != nil {
		log.Printf("Gob decode error inside HandleBlock: %v", err)
		return
	}

	block := DeserializeBlock(payload.Block)
	if block == nil {
		log.Printf("⚠️ [HandleBlock] Failed to deserialize block from %s. Dropping.", ShortID(peerID.String()))
		return
	}

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
			BroadcastBlock(s.BlockHub, block)
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
	// Cumulative cache: accumulates verified TXs across blocks so
	// cross-block dependencies within this IBD batch resolve in-memory.
	ibdTxCache := make(map[string]Transaction)

	for _, h := range heights {
		block := s.BlockBuffer[h]
		if block == nil {
			log.Printf("⚠️ [IBD] Nil block encountered at height %d, skipping...", h)
			continue
		}
		if s.Blockchain.AddBlock(block, ibdTxCache) {
			applied++
		}
	}

	fmt.Printf("✅ [IBD] Sync complete. Applied %d blocks.\n", applied)

	// Broadcast the tip block to WebSocket clients
	if len(heights) > 0 {
		if tipBlock := s.BlockBuffer[heights[len(heights)-1]]; tipBlock != nil {
			BroadcastBlock(s.BlockHub, tipBlock)
		}
	}

	// Full UTXO reindex from the now-complete chain
	fmt.Println("🔄 [IBD] Rebuilding UTXO set (Reindex)...")
	s.UTXOSet.Reindex()
	fmt.Println("✅ [IBD] UTXO Reindex complete.")
}

func (s *Server) HandleTx(request []byte, peerID peer.ID) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("⚡ Panic in HandleTx: %v", r)
		}
	}()

	var payload TxMsg
	dec := gob.NewDecoder(bytes.NewReader(request))
	if err := dec.Decode(&payload); err != nil {
		log.Printf("Gob decode error inside HandleTx: %v", err)
		return
	}

	txData := payload.Transaction
	tx := DeserializeTransaction(txData)

	s.MempoolMux.Lock()
	defer s.MempoolMux.Unlock()

	txID := hex.EncodeToString(tx.ID)
	if s.Mempool[txID].Tx.ID != nil {
		return
	}

	fee, err := s.UTXOSet.CalculateFee(&tx, s.Mempool)
	if err != nil {
		fmt.Printf("⚠️  [HandleTx] Rejected TX %x: Cannot calculate fee: %s\n", tx.ID, err)
		return
	}
	if fee < 0 {
		fmt.Printf("⚠️  [HandleTx] Rejected TX %x: Negative fee (%d)\n", tx.ID, fee)
		return
	}

	// Check for mempool double-spend: reject if any input is already consumed
	for _, vin := range tx.Vin {
		inputKey := hex.EncodeToString(vin.Txid) + ":" + fmt.Sprintf("%d", vin.Vout)
		for existingID, existing := range s.Mempool {
			if existingID == txID {
				continue
			}
			for _, evin := range existing.Tx.Vin {
				existingKey := hex.EncodeToString(evin.Txid) + ":" + fmt.Sprintf("%d", evin.Vout)
				if inputKey == existingKey {
					fmt.Printf("⚠️  [HandleTx] Rejected TX %x: double-spend attempt against mempool TX %s\n", tx.ID, existingID)
					return
				}
			}
		}
	}

	fmt.Printf("New Transaction in Mempool: %x (Fee: %d)\n", tx.ID, fee)
	s.Mempool[txID] = MempoolItem{Tx: tx, AddedAt: time.Now().Unix()}
	BroadcastMempoolTx(s.MempoolHub, &tx)

	peers := s.Host.Network().Peers()
	for _, p := range peers {
		if p != peerID {
			s.SendInv(p, "tx", [][]byte{tx.ID})
		}
	}
}

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

func (s *Server) AttemptMine() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("⚡ Panic in AttemptMine: %v", r)
			// Ensure map isn't locked indefinitely if it panics mid-mux
		}
	}()

	if s.MinerAddr == "" || s.ValidatorPrivKey == nil {
		return
	}

	s.MempoolMux.Lock()
	defer s.MempoolMux.Unlock()

	if len(s.Mempool) == 0 {
		return
	}

	fmt.Println("Forging new block with mempool transactions...")

	type txWithFee struct {
		tx  *Transaction
		fee int64
	}

	var validTxs []txWithFee
	var totalFees int64

	for id := range s.Mempool {
		item := s.Mempool[id]
		tx := item.Tx
		if s.Blockchain.VerifyTransactionWithMempool(&tx, s.Mempool) {
			fee, err := s.UTXOSet.CalculateFee(&tx, s.Mempool)
			if err == nil && fee >= 0 {
				validTxs = append(validTxs, txWithFee{tx: &tx, fee: fee})
			} else {
				// Invalid fee (or dependencies missing)
				delete(s.Mempool, id)
			}
		} else {
			delete(s.Mempool, id) // Clear invalid tx
		}
	}

	if len(validTxs) == 0 {
		fmt.Println("All transactions in mempool are invalid.")
		return
	}

	sort.Slice(validTxs, func(i, j int) bool {
		return validTxs[i].fee > validTxs[j].fee
	})

	var txs []*Transaction
	for _, twf := range validTxs {
		txs = append(txs, twf.tx)
		totalFees += twf.fee
	}

	bestHeight := s.Blockchain.GetBestHeight()
	nextHeight := bestHeight + 1
	subsidy := s.Blockchain.GetBlockSubsidy(nextHeight)

	totalReward := subsidy + totalFees
	cbTx := NewCoinbaseTX(s.MinerAddr, "", totalReward)

	// Detect and evict conflicting transactions instead of wiping the entire mempool
	prospectiveBlock := &Block{Transactions: append([]*Transaction{cbTx}, txs...)}
	if !s.UTXOSet.ValidateBlockTransactions(prospectiveBlock) {
		fmt.Println("⚠️  Mempool contains conflicting transactions. Evicting conflicts...")

		// Build a set of consumed inputs to detect conflicts
		spentInputs := make(map[string]string) // input outpoint -> first txID that claimed it
		var cleanTxs []txWithFee
		totalFees = 0

		for _, twf := range validTxs {
			conflict := false
			tid := hex.EncodeToString(twf.tx.ID)
			for _, vin := range twf.tx.Vin {
				key := hex.EncodeToString(vin.Txid) + ":" + fmt.Sprintf("%d", vin.Vout)
				if claimer, exists := spentInputs[key]; exists {
					fmt.Printf("  ↳ Evicted TX %s (conflicts with %s on input %s)\n", tid, claimer, key)
					delete(s.Mempool, tid)
					conflict = true
					break
				}
			}
			if !conflict {
				for _, vin := range twf.tx.Vin {
					key := hex.EncodeToString(vin.Txid) + ":" + fmt.Sprintf("%d", vin.Vout)
					spentInputs[key] = tid
				}
				cleanTxs = append(cleanTxs, twf)
				totalFees += twf.fee
			}
		}

		if len(cleanTxs) == 0 {
			fmt.Println("No valid transactions remain after conflict eviction.")
			return
		}

		// Rebuild the block with clean transactions
		totalReward = subsidy + totalFees
		cbTx = NewCoinbaseTX(s.MinerAddr, "", totalReward)
		txs = []*Transaction{cbTx}
		for _, twf := range cleanTxs {
			txs = append(txs, twf.tx)
		}
	} else {
		txs = append([]*Transaction{cbTx}, txs...) // Coinbase first
	}

	newBlock := s.Blockchain.ForgeBlock(txs, *s.ValidatorPrivKey)
	s.UTXOSet.Update(newBlock)
	BroadcastBlock(s.BlockHub, newBlock)

	s.Mempool = make(map[string]MempoolItem)

	fmt.Printf("New block forged: %x (Reward: %d | Sub: %d + Fee: %d)\n", newBlock.Hash, totalReward, subsidy, totalFees)

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

	// Write 4-byte big-endian length prefix
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	_, err = stream.Write(lenBuf)
	if err != nil {
		log.Printf("Error writing length prefix to %s: %v", ShortID(peerID.String()), err)
		return
	}

	_, err = stream.Write(data)
	if err != nil {
		log.Printf("Error writing payload to %s: %v", ShortID(peerID.String()), err)
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
