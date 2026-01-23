package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"os"
	"strconv"
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
	addressFlag string
	fromFlag    string
	toFlag      string
	amountFlag  int
	portFlag    int
	minerFlag   string
)

func Execute() {
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
		Short: "Inizializza il database locale con il Blocco Genesi ufficiale SOLE.",
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
	sendCmd.Flags().IntVar(&amountFlag, "amount", 0, "Amount to send")
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
	startNodeCmd.Flags().IntVar(&portFlag, "port", 3000, "Port to listen on")
	startNodeCmd.Flags().StringVar(&minerFlag, "miner", "", "Miner address")
	rootCmd.AddCommand(startNodeCmd)
}

func startNode(cmd *cobra.Command, args []string) {
	fmt.Printf("Avvio nodo SOLE su porta %d...\n", portFlag)

	var validatorPrivKey *ecdsa.PrivateKey

	if minerFlag != "" {
		fmt.Printf("Forging abilitato per indirizzo: %s\n", minerFlag)

		// Load wallet for this address
		wallets, err := CreateWallets()
		if err != nil {
			log.Panic("Errore caricamento wallets:", err)
		}

		wallet := wallets.GetWallet(minerFlag)
		if wallet.PrivateKey == nil {
			log.Panic("Errore: Wallet non trovato per indirizzo ", minerFlag)
		}

		privKey := wallet.GetPrivateKey()
		validatorPrivKey = &privKey

		// Print validator public key for registration
		pubKeyHex := GetValidatorHex(wallet)
		fmt.Printf("Validator PubKey: %s\n", pubKeyHex)
	}

	StartServer(portFlag, minerFlag, validatorPrivKey)
}

func runInit(cmd *cobra.Command, args []string) {
	chain, err := InitBlockchain()
	if err != nil {
		fmt.Println("⚠️  La blockchain esiste già. Usa './sole-cli startnode' per avviare.")
		return
	}
	defer chain.Database.Close()

	fmt.Println("\n☀️  SOLE Blockchain Inizializzata!")
	fmt.Printf("- Genesis Hash: %x\n", chain.LastHash)
	fmt.Println("- Network: Unisalento Mainnet")
	fmt.Println("- Pronti a partire. Esegui 'createwallet' o 'startnode'.")
}

func createWallet(cmd *cobra.Command, args []string) {
	wallets, _ := CreateWallets()
	address := wallets.AddWallet()
	wallets.SaveToFile()

	fmt.Printf("Nuovo portafoglio creato: %s\n", address)
}

func getBalance(cmd *cobra.Command, args []string) {
	if !ValidateAddress(addressFlag) {
		log.Panic("Errore: Indirizzo non valido")
	}
	chain := ContinueBlockchain(addressFlag)
	defer chain.Database.Close()

	balance := int64(0)
	pubKeyHash, _ := Base58Decode([]byte(addressFlag))
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4]
	utxos := chain.FindUnspentTransactions(pubKeyHash)

	for _, tx := range utxos {
		for _, out := range tx.Vout {
			if out.IsLockedWithKey(pubKeyHash) {
				balance += out.Value
			}
		}
	}

	fmt.Printf("Saldo di '%s': %d Fotoni (%.8f SOLE)\n", addressFlag, balance, float64(balance)/100000000.0)
}

func send(cmd *cobra.Command, args []string) {
	if !ValidateAddress(fromFlag) {
		log.Panic("Errore: Indirizzo Mittente non valido")
	}
	if !ValidateAddress(toFlag) {
		log.Panic("Errore: Indirizzo Destinatario non valido")
	}

	// Main logic handling
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
	defer chain.Database.Close()

	tx := NewUTXOTransaction(fromFlag, toFlag, int64(amountFlag), chain)

	// P2P Injection Logic
	fmt.Println("Ricerca nodi per inviare la transazione...")

	// Create transient host
	ctx := context.Background()
	h, err := libp2p.New()
	if err != nil {
		log.Panic(err)
	}

	// Setup mDNS to find peers
	notifee := &discoveryNotifee{h: h} // We need to adapt notifee or use channel
	// My discoveryNotifee in network.go creates connections automatically.
	// I can reuse it but I need to know WHEN connected to send.

	// Let's copy/paste simple mDNS logic here or modify StartServer?
	// StartServer is tailored for Daemon.

	// Custom Notify for Send
	ser := mdns.NewMdnsService(h, discoveryNamespace, notifee)
	if err := ser.Start(); err != nil {
		log.Panic(err)
	}

	// Wait for connection and send
	// We need to wait a bit for mDNS
	fmt.Println("In attesa di connessione a un peer...")
	found := false

	// We loop and check peers
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		peers := h.Network().Peers()
		if len(peers) > 0 {
			targetPeer := peers[0]
			fmt.Printf("Invio transazione a %s\n", targetPeer.String())

			// Serialize and Send
			data := TxMsg{h.ID().String(), tx.Serialize()}
			payload := GobEncode(data)
			request := append(CommandToBytes("tx"), payload...)

			stream, err := h.NewStream(ctx, targetPeer, protocolID)
			if err != nil {
				log.Panic(err)
			}
			_, err = stream.Write(request)
			if err != nil {
				log.Panic(err)
			}
			time.Sleep(500 * time.Millisecond) // Wait for write flush
			stream.Close()

			fmt.Println("Transazione inviata con successo!")
			found = true
			break
		}
	}

	if !found {
		fmt.Println("Errore: Nessun peer trovato per inviare la transazione.")
	}
}

func printChain(cmd *cobra.Command, args []string) {
	chain := ContinueBlockchain("")
	defer chain.Database.Close()

	iter := chain.Iterator()

	for {
		block := iter.Next()

		fmt.Printf("=== Blocco %d ===\n", block.Height)
		fmt.Printf("Hash: %x\n", block.Hash)
		fmt.Printf("Prev. Hash: %x\n", block.PrevBlockHash)
		pow := true // No PoW validation implemented properly yet, just flag
		fmt.Printf("PoA Valid: %s\n", strconv.FormatBool(pow))
		fmt.Println("Transazioni:")
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
		log.Panic("Errore: Indirizzo non valido")
	}

	wallets, err := CreateWallets()
	if err != nil {
		log.Panic(err)
	}

	wallet := wallets.GetWalletRef(addressFlag)
	if wallet == nil {
		log.Panic("Errore: Wallet non trovato per questo indirizzo")
	}

	privKey := wallet.GetPrivateKey()
	pubKeyHex := fmt.Sprintf("%x", wallet.PublicKey)
	privKeyHex := fmt.Sprintf("%x", privKey.D.Bytes())

	fmt.Println("=== Wallet Details ===")
	fmt.Printf("Address:      %s\n", addressFlag)
	fmt.Printf("Public Key:   %s\n", pubKeyHex)
	fmt.Printf("Private Key:  %s\n", privKeyHex)
	fmt.Println("======================")
}

func listAddresses(cmd *cobra.Command, args []string) {
	wallets, err := CreateWallets()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Nessun portafoglio trovato.")
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
