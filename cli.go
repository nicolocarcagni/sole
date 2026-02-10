package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "sole-cli",
	Short: "SOLE Blockchain CLI",
	Long:  `Line command interface for SOLE Blockchain (Educational Project).`,
}

// Flags variables
var (
	addressFlag   string
	fromFlag      string
	toFlag        string
	amountFlag    float64
	portFlag      int
	minerFlag     string
	apiPortFlag   int
	dryRunFlag    bool
	listenFlag    string // Bind Address (0.0.0.0)
	publicIPFlag  string // Announce Address
	publicDNSFlag string // Announce Domain (node.sole.com)
	bootnodesFlag string // Comma-separated bootnodes
	apiListenFlag string // API Bind Address (0.0.0.0)
	privKeyFlag   string // Private Key Hex for import
)

func Execute() {
	// Default to Help if no args provided
	if len(os.Args) < 2 {
		rootCmd.Help()
		os.Exit(0)
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// createblockchain
	// init (formerly createblockchain)
	var initCmd = &cobra.Command{
		Use:   "init",
		Short: "Initializes the local database with the Official Genesis Block.",
		Run:   runInit,
	}
	rootCmd.AddCommand(initCmd)

	// createwallet
	var createWalletCmd = &cobra.Command{
		Use:   "createwallet",
		Short: "Create a new wallet",
		Run:   createWallet,
	}
	rootCmd.AddCommand(createWalletCmd)

	// importwallet
	var importWalletCmd = &cobra.Command{
		Use:   "importwallet",
		Short: "Imports a wallet from a Hex Private Key",
		Run:   runImportWallet,
	}
	importWalletCmd.Flags().StringVar(&privKeyFlag, "privkey", "", "Private Key in Hex format")
	importWalletCmd.MarkFlagRequired("privkey")
	rootCmd.AddCommand(importWalletCmd)

	// getbalance
	// getbalance
	var getBalanceCmd = &cobra.Command{
		Use:   "getbalance",
		Short: "Get balance of an address",
		Run:   getBalance,
	}
	getBalanceCmd.Flags().StringVar(&addressFlag, "address", "", "Address to check balance for")
	getBalanceCmd.MarkFlagRequired("address")
	rootCmd.AddCommand(getBalanceCmd)

	// printwallet
	var printWalletCmd = &cobra.Command{
		Use:   "printwallet",
		Short: "Print wallet details (Private Key)",
		Run:   printWallet,
	}
	printWalletCmd.Flags().StringVar(&addressFlag, "address", "", "Address to print")
	printWalletCmd.MarkFlagRequired("address")
	rootCmd.AddCommand(printWalletCmd)

	// listaddresses
	var listAddressesCmd = &cobra.Command{
		Use:   "listaddresses",
		Short: "Lists all addresses in the local wallet file",
		Run:   listAddresses,
	}
	rootCmd.AddCommand(listAddressesCmd)

	// send
	var sendCmd = &cobra.Command{
		Use:   "send",
		Short: "Send amount from one address to another",
		Run:   send,
	}
	sendCmd.Flags().StringVar(&fromFlag, "from", "", "Source address")
	sendCmd.Flags().StringVar(&toFlag, "to", "", "Destination address")
	sendCmd.Flags().Float64Var(&amountFlag, "amount", 0, "Amount to send")
	sendCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Print transaction hex without sending")
	sendCmd.MarkFlagRequired("from")
	sendCmd.MarkFlagRequired("to")
	sendCmd.MarkFlagRequired("amount")
	rootCmd.AddCommand(sendCmd)

	// printchain
	var printChainCmd = &cobra.Command{
		Use:   "printchain",
		Short: "Print all blocks in the chain",
		Run:   printChain,
	}
	rootCmd.AddCommand(printChainCmd)

	// startnode
	var startNodeCmd = &cobra.Command{
		Use:   "startnode",
		Short: "Start the P2P node",
		Run:   startNode,
	}
	// reindex
	var reindexCmd = &cobra.Command{
		Use:   "reindex",
		Short: "Rebuilds the UTXO set",
		Run:   reindexUTXO,
	}
	rootCmd.AddCommand(reindexCmd)

	startNodeCmd.Flags().IntVar(&portFlag, "port", 3000, "P2P Port")
	startNodeCmd.Flags().StringVar(&listenFlag, "listen", "0.0.0.0", "Local Listen IP for P2P")
	startNodeCmd.Flags().StringVar(&publicIPFlag, "public-ip", "", "Public IP Address (Announce)")
	startNodeCmd.Flags().StringVar(&publicDNSFlag, "public-dns", "", "Public Domain Name (Announce)")
	startNodeCmd.Flags().StringVar(&bootnodesFlag, "bootnodes", "", "Comma-separated list of Bootnodes")
	startNodeCmd.Flags().StringVar(&minerFlag, "miner", "", "Miner address")
	startNodeCmd.Flags().IntVar(&apiPortFlag, "api-port", 8080, "API Server Port")
	startNodeCmd.Flags().StringVar(&apiListenFlag, "api-listen", "0.0.0.0", "Local Listen IP for API")
	rootCmd.AddCommand(startNodeCmd)
}

func startNode(cmd *cobra.Command, args []string) {
	fmt.Printf("Starting SOLE node on port %d...\n", portFlag)

	// Check DB existence if not mining (or even if mining, usually need DB)
	// But ContinueBlockchain inside StartServer or Network will handle it?
	// The request asked for check in startnode.
	if !DBExists() {
		fmt.Println("⚠️  Database not found. Did you run './sole-cli init'?")
		os.Exit(1)
	}

	var validatorPrivKey *ecdsa.PrivateKey

	if minerFlag != "" {
		fmt.Printf("Forging enabled for address: %s\n", minerFlag)

		// Load wallet for this address
		wallets, err := CreateWallets()
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("⛔ ERROR: Private Key not found for address %s. Wallet file missing.\n", minerFlag)
				os.Exit(1)
			}
			log.Panic("Error loading wallets:", err)
		}

		wallet := wallets.GetWalletRef(minerFlag)
		if wallet == nil {
			fmt.Printf("⛔ ERROR: Private Key not found for address %s. Cannot mine without owning the wallet.\n", minerFlag)
			os.Exit(1)
		}

		privKey := wallet.GetPrivateKey()
		validatorPrivKey = &privKey

		// Print validator public key for registration
		pubKeyHex := GetValidatorHex(*wallet)
		fmt.Printf("Validator PubKey: %s\n", pubKeyHex)

		// Authorization Check
		if !IsAuthorizedValidator(pubKeyHex) {
			fmt.Printf("⛔ ERROR: Address %s is not an Authorized Validator. Mining aborted.\n", minerFlag)
			os.Exit(1)
		}
		fmt.Println("✅ Authorized Validator recognized. Starting Consensus Engine...")
	}

	// Parse bootnodes
	var bootnodes []string
	if bootnodesFlag != "" {
		bootnodes = strings.Split(bootnodesFlag, ",")
	}

	// Load Persistent P2P Identity
	nodeKeyPath := "node_key.dat"
	privKeyP2P, err := LoadOrGenerateNodeKey(nodeKeyPath)
	if err != nil {
		log.Panic("Error loading node key:", err)
	}

	// Config
	cfg := ServerConfig{
		ListenHost: listenFlag,
		Port:       portFlag,
		PublicIP:   publicIPFlag,
		PublicDNS:  publicDNSFlag,
		Bootnodes:  bootnodes,
		MinerAddr:  minerFlag,
		PrivKey:    validatorPrivKey,
		NodeKey:    privKeyP2P,
	}

	// Initialize P2P Server
	server := NewServer(cfg)
	// We handle DB closing manually on signal
	// defer server.Blockchain.Database.Close()

	// Start API Server
	go StartRestServer(server, apiListenFlag, apiPortFlag)

	// Start P2P Loop (in background)
	go server.Start()

	// Start Periodic Mining Loop (if miner)
	if minerFlag != "" {
		go server.StartMiningLoop()
	}

	// Graceful Shutdown Handling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	<-stop // Block here until signal received

	fmt.Println("\n⚠️  Stop signal received. Shutting down...")

	// 1. Close P2P Host (Network)
	if err := server.Host.Close(); err != nil {
		fmt.Printf("Error closing P2P Host: %s\n", err)
	}

	// 2. Close Database (Persistence)
	// Important: This releases the LOCK file
	if err := server.Blockchain.Database.Close(); err != nil {
		fmt.Printf("Error closing Database: %s\n", err)
	}

	fmt.Println("✅ Node shut down correctly. See you soon!")
}

func runInit(cmd *cobra.Command, args []string) {
	if DBExists() {
		fmt.Println("⚠️  Blockchain already exists. Use './sole-cli startnode' to start.")
		return
	}

	chain, err := InitBlockchain()
	if err != nil {
		fmt.Printf("⚠️  Error initializing blockchain: %s\n", err)
		return
	}
	defer chain.Database.Close()

	// Auto-Reindex UTXO Set
	UTXOSet := UTXOSet{chain}
	UTXOSet.Reindex()

	fmt.Println("\n☀️  SOLE Blockchain Initialized!")
	fmt.Printf("- Genesis Hash: %x\n", chain.LastHash)
	fmt.Println("- Network: Unisalento Mainnet")
	fmt.Println("- UTXO Set: Reindexed automatically.")
	fmt.Println("- Run 'createwallet' or 'startnode'.")
}

func createWallet(cmd *cobra.Command, args []string) {
	wallets, _ := CreateWallets()
	address := wallets.AddWallet()
	wallets.SaveToFile()

	fmt.Printf("New wallet created: %s\n", address)
}

func runImportWallet(cmd *cobra.Command, args []string) {
	wallets, _ := CreateWallets()
	address, err := wallets.ImportWallet(privKeyFlag)
	if err != nil {
		log.Panic(err)
	}

	wallets.SaveToFile()

	fmt.Printf("Success! Wallet imported. Address: %s\n", address)
}

func getBalance(cmd *cobra.Command, args []string) {
	if !ValidateAddress(addressFlag) {
		fmt.Println("⛔ ERROR: Invalid address provided.")
		os.Exit(1)
	}
	chain := ContinueBlockchain(addressFlag)
	UTXOSet := UTXOSet{chain}
	defer chain.Database.Close()

	balance := int64(0)
	pubKeyHash, _ := Base58Decode([]byte(addressFlag))
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4]
	utxos := UTXOSet.FindUnspentOutputs(pubKeyHash)

	for _, out := range utxos {
		balance += out.Value
	}

	fmt.Printf("Balance of '%s': %d Photons (%.8f SOLE)\n", addressFlag, balance, float64(balance)/100000000.0)
}

func send(cmd *cobra.Command, args []string) {
	if !ValidateAddress(fromFlag) {
		fmt.Println("⛔ ERROR: Invalid sender address.")
		os.Exit(1)
	}
	if !ValidateAddress(toFlag) {
		fmt.Println("⛔ ERROR: Invalid recipient address.")
		os.Exit(1)
	}
	if amountFlag <= 0 {
		fmt.Println("⛔ ERROR: Amount must be greater than zero.")
		os.Exit(1)
	}

	// Main logic handling
	// Workaround for DB Lock: Create a snapshot copy of the DB
	snapshotPath := dbPath + "_snapshot_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	err := CopyDir(dbPath, snapshotPath)
	if err != nil {
		log.Panic("Failed to create DB snapshot:", err)
	}
	defer os.RemoveAll(snapshotPath) // Cleanup

	// Open snapshot
	chain := ContinueBlockchainSnapshot(snapshotPath)
	UTXOSet := UTXOSet{chain}
	defer chain.Database.Close()

	// Conversion: SOLE (Float) -> Photons (Int64)
	amountInt := int64(amountFlag * 100000000)
	fmt.Printf("💸 Sending: %.8f SOLE (%d Photons)\n", amountFlag, amountInt)

	tx := NewUTXOTransaction(fromFlag, toFlag, amountInt, &UTXOSet)

	if dryRunFlag {
		fmt.Printf("Dry-Run: Transaction Hex:\n%x\n", tx.Serialize())
		return
	}

	// P2P Injection Logic
	fmt.Println("Searching for peers to broadcast transaction...")

	// Panic Recovery
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("⚠️  Recuperato da Panic in 'send': %v\n", r)
		}
	}()

	// Create transient host
	ctx := context.Background()
	h, err := libp2p.New()
	if err != nil {
		log.Panic(err)
	}
	defer h.Close()

	// Setup mDNS to find peers
	// Note: We pass nil as server because we are just a transient client.
	// HandlePeerFound checks for nil server now.
	notifee := &discoveryNotifee{h: h, server: nil}
	ser := mdns.NewMdnsService(h, discoveryNamespace, notifee)
	if err := ser.Start(); err != nil {
		log.Panic(err)
	}

	// Wait for connection and send
	// Wait for connection and send
	fmt.Println("Waiting for connection...")
	found := false
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Loop Event Detection
	for {
		select {
		case <-timeout:
			fmt.Println("⏰ Timeout: No peers found within 10 seconds. Is a node running?")
			return
		case <-ticker.C:
			peers := h.Network().Peers()
			if len(peers) > 0 {
				targetPeer := peers[0]
				// Avoid self? (Though CLI has different ID than Node usually, unless sharing key)

				fmt.Printf("Sending transaction to %s\n", targetPeer.String())

				// Serialize and Send
				data := TxMsg{h.ID().String(), tx.Serialize()}
				payload := GobEncode(data)
				request := append(CommandToBytes("tx"), payload...)

				stream, err := h.NewStream(ctx, targetPeer, protocolID)
				if err != nil {
					fmt.Printf("⚠️  Error opening stream: %s\n", err)
					continue
				}

				_, err = stream.Write(request)
				if err != nil {
					fmt.Printf("⚠️  Error sending data: %s\n", err)
					stream.Close()
					continue
				}
				time.Sleep(500 * time.Millisecond) // Wait for write flush
				stream.Close()

				fmt.Println("✅ Transaction sent successfully!")
				found = true
				goto END_LOOP
			}
		}
	}

END_LOOP:
	if !found {
		fmt.Println("Error: No peers found to broadcast transaction.")
	}
}

func printChain(cmd *cobra.Command, args []string) {
	chain := ContinueBlockchain("")
	defer chain.Database.Close()

	iter := chain.Iterator()

	for {
		block := iter.Next()

		fmt.Printf("=== Block %d ===\n", block.Height)
		fmt.Printf("Hash: %x\n", block.Hash)
		fmt.Printf("Prev. Hash: %x\n", block.PrevBlockHash)
		pow := true // No PoW validation implemented properly yet, just flag
		fmt.Printf("PoA Valid: %s\n", strconv.FormatBool(pow))
		fmt.Println("Transactions:")
		for _, tx := range block.Transactions {
			fmt.Printf("  TX ID: %x\n", tx.ID)
		}
		fmt.Println()

		if len(block.PrevBlockHash) == 0 {
			break
		}
	}
}

func printWallet(cmd *cobra.Command, args []string) {
	if !ValidateAddress(addressFlag) {
		log.Panic("Error: Invalid Address")
	}

	wallets, err := CreateWallets()
	if err != nil {
		log.Panic(err)
	}

	wallet := wallets.GetWalletRef(addressFlag)
	if wallet == nil {
		log.Panic("Error: Wallet not found for this address")
	}

	privKey := wallet.GetPrivateKey()
	// Using hex.EncodeToString as requested for clarity
	pubKeyHex := hex.EncodeToString(wallet.PublicKey)
	privKeyHex := hex.EncodeToString(privKey.D.Bytes())

	fmt.Println("=== Wallet Details ===")
	fmt.Printf("Address:          %s\n", addressFlag)
	fmt.Printf("Public Key (Hex): %s\n", pubKeyHex)
	fmt.Printf("Private Key:      %s\n", privKeyHex)
	fmt.Println("======================")
}

func listAddresses(cmd *cobra.Command, args []string) {
	wallets, err := CreateWallets()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No wallets found.")
			return
		}
		log.Panic(err)
	}
	addresses := wallets.GetAddresses()

	fmt.Println("=== Local Wallets ===")
	for _, address := range addresses {
		fmt.Println(address)
	}
	fmt.Println("=====================")
}

func reindexUTXO(cmd *cobra.Command, args []string) {
	chain := ContinueBlockchain("")
	defer chain.Database.Close()

	UTXOSet := UTXOSet{chain}
	UTXOSet.Reindex()

	count := UTXOSet.CountTransactions()
	fmt.Printf("✅ Reindexing completed! There are %d transactions in the UTXO set.\n", count)
}
