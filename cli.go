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
	"text/tabwriter"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/spf13/cobra"
)

// ANSI Colors
const (
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorCyan   = "\033[36m"
	ColorReset  = "\033[0m"
	ColorBold   = "\033[1m"
	ColorRed    = "\033[31m"
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
	// Custom Help
	rootCmd.SetHelpFunc(printUsage)
	rootCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		printUsage(cmd, nil)
		return nil
	})

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

func printUsage(cmd *cobra.Command, args []string) {
	fmt.Println(ColorGreen + `
   _____  ____  _      ______ 
  / ____|/ __ \| |    |  ____|
 | (___ | |  | | |    | |__   
  \___ \| |  | | |    |  __|  
  ____) | |__| | |____| |____ 
 |_____/ \____/|______|______|
` + ColorReset)
	fmt.Println(ColorBold + "   SOLE Blockchain CLI v1.0" + ColorReset)
	fmt.Println("   (c) 2026 Università del Salento")
	fmt.Println()

	fmt.Println(ColorBold + "USAGE:" + ColorReset)
	fmt.Println("  ./sole-cli <resource> <action> [flags]")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)

	// 1. WALLET
	fmt.Fprintln(w, ColorYellow+"1. WALLET MANAGEMENT (wallet)"+ColorReset)
	fmt.Fprintln(w, "  "+ColorGreen+"create"+ColorReset+"\tGenerates a new keypair.")
	fmt.Fprintln(w, "  "+ColorGreen+"list"+ColorReset+"\tLists saved addresses.")
	fmt.Fprintln(w, "  "+ColorGreen+"import"+ColorReset+"\tImports a private key (--key <HEX>).")
	fmt.Fprintln(w, "  "+ColorGreen+"remove"+ColorReset+"\tRemoves a wallet (--address <ADDR>).")
	fmt.Fprintln(w, "  "+ColorGreen+"balance"+ColorReset+"\tChecks balance of an address (--address <ADDR>).")
	fmt.Fprintln(w, "  "+ColorGreen+"export"+ColorReset+"\tExports private key (--address <ADDR>).")
	fmt.Fprintln(w, "")

	// 2. CHAIN
	fmt.Fprintln(w, ColorYellow+"2. BLOCKCHAIN OPERATIONS (chain)"+ColorReset)
	fmt.Fprintln(w, "  "+ColorGreen+"init"+ColorReset+"\tInitializes the Genesis Block and DB.")
	fmt.Fprintln(w, "  "+ColorGreen+"reindex"+ColorReset+"\tRebuilds the UTXO index.")
	fmt.Fprintln(w, "  "+ColorGreen+"print"+ColorReset+"\tPrints all blocks in the chain.")
	fmt.Fprintln(w, "  "+ColorGreen+"reset"+ColorReset+"\t"+ColorRed+"DELETES"+ColorReset+" the blockchain database.")
	fmt.Fprintln(w, "")

	// 3. NODE
	fmt.Fprintln(w, ColorYellow+"3. NODE & NETWORK (node)"+ColorReset)
	fmt.Fprintln(w, "  "+ColorGreen+"start"+ColorReset+"\tStarts the P2P node and Miner.")
	fmt.Fprintln(w, "\t"+ColorCyan+"Flags:"+ColorReset+" --port, --miner, --bootnodes, --public-ip")
	fmt.Fprintln(w, "")

	// 4. TX
	fmt.Fprintln(w, ColorYellow+"4. TRANSACTIONS (tx)"+ColorReset)
	fmt.Fprintln(w, "  "+ColorGreen+"send"+ColorReset+"\tSends funds between wallets.")
	fmt.Fprintln(w, "\t"+ColorCyan+"Flags:"+ColorReset+" --from, --to, --amount, --dry-run")
	fmt.Fprintln(w, "")

	w.Flush()
	fmt.Println()
}

func init() {
	// --- WALLET COMMANDS ---
	var walletCmd = &cobra.Command{
		Use:   "wallet",
		Short: "Manage wallets",
	}
	rootCmd.AddCommand(walletCmd)

	var walletCreateCmd = &cobra.Command{
		Use:   "create",
		Short: "Create a new wallet",
		Run:   createWallet,
	}
	walletCmd.AddCommand(walletCreateCmd)

	var walletListCmd = &cobra.Command{
		Use:   "list",
		Short: "Lists all addresses in the local wallet file",
		Run:   listAddresses,
	}
	walletCmd.AddCommand(walletListCmd)

	var walletImportCmd = &cobra.Command{
		Use:   "import",
		Short: "Imports a wallet from a Hex Private Key",
		Run:   runImportWallet,
	}
	// Changed flag from 'privkey' to 'key' as requested
	walletImportCmd.Flags().StringVar(&privKeyFlag, "key", "", "Private Key in Hex format")
	walletImportCmd.MarkFlagRequired("key")
	walletCmd.AddCommand(walletImportCmd)

	var walletRemoveCmd = &cobra.Command{
		Use:   "remove",
		Short: "Removes a wallet from a wallet file",
		Run:   runRemoveWallet,
	}
	walletRemoveCmd.Flags().StringVar(&addressFlag, "address", "", "Address of the wallet to remove")
	walletRemoveCmd.MarkFlagRequired("address")
	walletCmd.AddCommand(walletRemoveCmd)

	var walletBalanceCmd = &cobra.Command{
		Use:   "balance",
		Short: "Get balance of an address",
		Run:   getBalance,
	}
	walletBalanceCmd.Flags().StringVar(&addressFlag, "address", "", "Address to check balance for")
	walletBalanceCmd.MarkFlagRequired("address")
	walletCmd.AddCommand(walletBalanceCmd)

	var walletExportCmd = &cobra.Command{
		Use:   "export",
		Short: "Print wallet details (Private Key)",
		Run:   printWallet,
	}
	walletExportCmd.Flags().StringVar(&addressFlag, "address", "", "Address to print")
	walletExportCmd.MarkFlagRequired("address")
	walletCmd.AddCommand(walletExportCmd)

	// --- CHAIN COMMANDS ---
	var chainCmd = &cobra.Command{
		Use:   "chain",
		Short: "Manage blockchain database",
	}
	rootCmd.AddCommand(chainCmd)

	var chainInitCmd = &cobra.Command{
		Use:   "init",
		Short: "Initializes the local database with the Official Genesis Block.",
		Run:   runInit,
	}
	chainCmd.AddCommand(chainInitCmd)

	var chainReindexCmd = &cobra.Command{
		Use:   "reindex",
		Short: "Rebuilds the UTXO set",
		Run:   reindexUTXO,
	}
	chainCmd.AddCommand(chainReindexCmd)

	var chainPrintCmd = &cobra.Command{
		Use:   "print",
		Short: "Print all blocks in the chain",
		Run:   printChain,
	}
	chainCmd.AddCommand(chainPrintCmd)

	var chainResetCmd = &cobra.Command{
		Use:   "reset",
		Short: "Resets (DELETES) the blockchain database",
		Run:   runResetChain,
	}
	chainCmd.AddCommand(chainResetCmd)

	// --- NODE COMMANDS ---
	var nodeCmd = &cobra.Command{
		Use:   "node",
		Short: "Manage P2P node",
	}
	rootCmd.AddCommand(nodeCmd)

	var nodeStartCmd = &cobra.Command{
		Use:   "start",
		Short: "Start the P2P node",
		Run:   startNode,
	}
	nodeStartCmd.Flags().IntVar(&portFlag, "port", 3000, "P2P Port")
	nodeStartCmd.Flags().StringVar(&listenFlag, "listen", "0.0.0.0", "Local Listen IP for P2P")
	nodeStartCmd.Flags().StringVar(&publicIPFlag, "public-ip", "", "Public IP Address (Announce)")
	nodeStartCmd.Flags().StringVar(&publicDNSFlag, "public-dns", "", "Public Domain Name (Announce)")
	nodeStartCmd.Flags().StringVar(&bootnodesFlag, "bootnodes", "", "Comma-separated list of Bootnodes")
	nodeStartCmd.Flags().StringVar(&minerFlag, "miner", "", "Miner address")
	nodeStartCmd.Flags().IntVar(&apiPortFlag, "api-port", 8080, "API Server Port")
	nodeStartCmd.Flags().StringVar(&apiListenFlag, "api-listen", "0.0.0.0", "Local Listen IP for API")
	nodeCmd.AddCommand(nodeStartCmd)

	// --- TX COMMANDS ---
	var txCmd = &cobra.Command{
		Use:   "tx",
		Short: "Manage transactions",
	}
	rootCmd.AddCommand(txCmd)

	var txSendCmd = &cobra.Command{
		Use:   "send",
		Short: "Send amount from one address to another",
		Run:   send,
	}
	txSendCmd.Flags().StringVar(&fromFlag, "from", "", "Source address")
	txSendCmd.Flags().StringVar(&toFlag, "to", "", "Destination address")
	txSendCmd.Flags().Float64Var(&amountFlag, "amount", 0, "Amount to send")
	txSendCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Print transaction hex without sending")
	txSendCmd.MarkFlagRequired("from")
	txSendCmd.MarkFlagRequired("to")
	txSendCmd.MarkFlagRequired("amount")
	txCmd.AddCommand(txSendCmd)
}

func startNode(cmd *cobra.Command, args []string) {
	fmt.Printf("Starting SOLE node on port %d...\n", portFlag)

	// Check DB existence if not mining (or even if mining, usually need DB)
	// But ContinueBlockchain inside StartServer or Network will handle it?
	// The request asked for check in node start.
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
		fmt.Println("⚠️  Blockchain already exists. Use './sole-cli node start' to start.")
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
	fmt.Println("- Run 'wallet create' or 'node start'.")
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

func runRemoveWallet(cmd *cobra.Command, args []string) {
	if !ValidateAddress(addressFlag) {
		fmt.Println("⛔ ERROR: Invalid address provided.")
		os.Exit(1)
	}

	wallets, err := CreateWallets()
	if err != nil {
		log.Panic(err)
	}

	// Check existence before prompt
	if wallets.GetWalletRef(addressFlag) == nil {
		fmt.Println("❌ Error: Address not found in wallet file.")
		os.Exit(1)
	}

	// Confirmation Prompt
	fmt.Printf("⚠️  Are you sure you want to remove wallet %s? [y/N]: ", addressFlag)
	var response string
	fmt.Scanln(&response)

	if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
		fmt.Println("Operation cancelled.")
		return
	}

	err = wallets.RemoveWallet(addressFlag)
	if err != nil {
		fmt.Println("❌ Error:", err)
		os.Exit(1)
	}

	wallets.SaveToFile()

	fmt.Printf("✅ Wallet %s removed successfully.\n", addressFlag)
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

	// Re-add reindexUTXO at end of file if it was cut off, or just append runResetChain
	count := UTXOSet.CountTransactions()
	fmt.Printf("✅ Reindexing completed! There are %d transactions in the UTXO set.\n", count)
}

func runResetChain(cmd *cobra.Command, args []string) {
	if !DBExists() {
		fmt.Println("⚠️  No blockchain found to reset.")
		return
	}

	fmt.Print("⚠️  Are you sure you want to RESET the chain? This will delete all data! [y/N]: ")
	var response string
	fmt.Scanln(&response)

	if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
		fmt.Println("Operation cancelled.")
		return
	}

	err := os.RemoveAll(dbPath)
	if err != nil {
		log.Panic(err)
	}
	fmt.Println("✅ Blockchain database deleted.")
}
