# SOLE Blockchain ☀️

> **⚠️ DISCLAIMER: EDUCATIONAL PROJECT**
> This is an independent, personal project developed for educational and research purposes only.
> It is **NOT** an official product of the **University of Salento (Unisalento)** nor is it associated with its administration.
> All references to the university are contextual for the case study simulation.

**SOLE** (SLN) is an educational blockchain project designed to create a digital token for the **University of Salento** (Università del Salento). The project facilitates internal transactions and services within the university campus, serving as a practical case study for blockchain technology.

Built in Go, SOLE features a custom **Proof of Authority (PoA)** consensus mechanism, P2P networking using `libp2p`, and efficient persistence via `BadgerDB`.

## 🌟 Features

*   **Proof of Authority (PoA) Consensus**: Blocks are forged by authorized validators using ECDSA signatures, replacing energy-intensive mining.
*   **P2P Networking**: Fully decentralized peer discovery and synchronization using `libp2p` (mDNS & DHT support).
*   **Persistent Storage**: Fast and reliable key-value storage using `BadgerDB` with memory optimizations for low-end hardware.
*   **Wallet System**: Secure wallet management with ECDSA key pairs and Address generation (Base58Check).
*   **CLI Interface**: Robust command-line tool for interacting with the blockchain (wallet management, sending transactions, node operation).
*   **UTXO Model**: Bitcoin-like Unspent Transaction Output model for tracking balances.
*   **REST API**: Integrated HTTP Gateway (`gorilla/mux`) for external application interaction.

## 🏗 Architecture

### Consensus Logic
SOLE uses a Proof of Authority model where only specific nodes (Validators) holding authorized private keys can create (forge) new blocks.
*   **Forging**: Validators sign the block hash with their private key.
*   **Validation**: Nodes verify the `Validator` public key and the `Signature` against the authorized list defined in `consensus.go`.

### Networking
*   **Protocol**: Custom binary protocol over `libp2p` streams.
*   **Sync**: Automatic blockchain synchronization upon connection (Version handshake -> Inventory -> GetBlocks -> GetData).
*   **Mempool**: Transactions are broadcasted to peers and stored in a memory pool until forged into a block.

### Persistence
Data is stored in `BadgerDB`, optimized for low memory footprint (~25MB RAM usage):
*   `ValueLogFileSize`: 16MB
*   `MemTableSize`: 8MB
*   `BlockCacheSize`: 1MB

## 🚀 Getting Started

### Prerequisites
*   Go 1.22 or higher

### Installation
Clone the repository and build the CLI tool:

```bash
git clone https://github.com/nicolocarcagni/sole.git
cd sole
go build -o sole-cli .
```

## 📖 Usage

The `sole-cli` tool is the main entry point.

### 1. Create a Wallet
Generate a new wallet and address. Access to private keys is stored locally in `wallet.dat`.
```bash
./sole-cli createwallet
```

### 2. Initialize Blockchain
Initialize the blockchain with the **Hardcoded Genesis Block**.
*   **Note**: The Genesis Block acts as the immutable anchor of the network. It pays the initial supply (Premine) to the **Admin Address** hardcoded in `genesis.go`.
*   **Usage**: No arguments required.
```bash
./sole-cli init
```

### 3. Start a Node
Start a P2P node.
*   **Full Node**: Synchronizes with the network.
*   **Validator Node**: Forges new blocks (requires `--miner` flag with a validator address).

**Start a regular node:**
```bash
./sole-cli startnode --port 3000
```

**Start a Validator node (Forging):**
*Note: The address must be authorized in `consensus.go`.*
```bash
./sole-cli startnode --port 3000 --miner <VALIDATOR_ADDRESS>
```

### 4. Send Transactions
Send coins from one address to another.
```bash
./sole-cli send --from <SENDER> --to <RECEIVER> --amount <AMOUNT>
```

### 5. Check Balance
View the balance of an address.
```bash
./sole-cli getbalance --address <ADDRESS>
```

### 6. Inspect Blockchain
Print the blocks in the chain.
```bash
./sole-cli printchain
```

### 7. Export Private Keys
Export your wallet's Private Key (in Hex format) for backup or Validator configuration.
```bash
./sole-cli printwallet --address <ADDRESS>
```

### 8. REST API Gateway 🌍
Interact with the node via HTTP.
*   **Enable**: Add `--api-port <PORT>` to `startnode`.
*   **Default**: Port 8080.
```bash
./sole-cli startnode --api-port 8080
```
*   **Endpoints**:
    *   `GET /blocks/tip` (Status)
    *   `GET /balance/{address}`
    *   `POST /tx/send`
*   👉 **[Full API Documentation](docs/api_reference.md)**

## 🔧 Configuration

### Proof of Authority (PoA) Configuration
The PoA mechanism relies on a static list of authorized public keys. This ensures that only trusted nodes can forge blocks.

#### 1. Hardcoded Genesis Authority
The **Genesis Block** is mathematically hardcoded in `genesis.go` to ensure all nodes start from the exact same state.
*   **Admin Address**: The address that receives the initial token supply (Premine) is defined by the constant `GenesisAdminAddress`.
*   **Coinbase Data**: The arbitrary data in the genesis transaction is fixed to `"Lu sule, lu mare, lu ientu. Unisalento 2026."`.

#### 2. Adding New Validators
To authorize a new node to forge blocks:
1.  **Generate a Wallet** on the new node: `./sole-cli createwallet`.
2.  **Get the Public Key**: Use `./sole-cli printwallet --address <NEW_ADDR>` to see the `Public Key` (Hex).
3.  **Update Source Code**: Add this Hex string to the `AuthorizedValidators` list in `consensus.go`.
4.  **Rebuild**: Recompile the project (`go build`) and distribute the new binary to all nodes.

```go
// consensus.go
var AuthorizedValidators = []string{
    "EXISTING_ADMIN_PUBKEY_HEX",
    "NEW_VALIDATOR_PUBKEY_HEX",
}
```

## 🤝 Contributing
Contributions are welcome! Please feel free to submit a Pull Request.

## 📜 License
This project is open-source and available under the MIT License.