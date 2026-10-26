<div align="center">

# SOLE Blockchain
### **Secure Open Ledger for Education**

*An educational Proof-of-Authority (PoA) blockchain developed in Go for the University of Salento.*

<p align="center">
  <a href="https://golang.org"><img src="https://img.shields.io/badge/Go-1.22%2B-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go Version" /></a>
  <a href="https://github.com/nicolocarcagni/sole/releases"><img src="https://img.shields.io/badge/Release-v4.0.0-blue?style=flat-square&logo=github" alt="Release v4.0.0" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-green.svg?style=flat-square" alt="License: MIT" /></a>
  <a href="https://www.unisalento.it"><img src="https://img.shields.io/badge/UniSalento-Academic%20Project-E30613?style=flat-square&logo=academia&logoColor=white" alt="UniSalento" /></a>
  <a href="https://github.com/nicolocarcagni/sole/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/nicolocarcagni/sole/ci.yml?branch=main&style=flat-square&logo=githubactions&logoColor=white&label=CI" alt="CI Status" /></a>
  <a href="https://github.com/nicolocarcagni/sole"><img src="https://img.shields.io/badge/Go%20Report-A%2B-brightgreen?style=flat-square&logo=go&logoColor=white" alt="Go Report Card" /></a>
</p>

<p align="center">
  <strong>Official Bootnode:</strong> <code>sole.nicolocarcagni.dev:3000</code> &nbsp;|&nbsp; 
  <strong>Network:</strong> <code>UniSalento PoA Testnet</code>
</p>

---

[About](#about) •
[Features](#features) •
[Specifications](#specifications) •
[Architecture](#architecture) •
[Quickstart](#quickstart) •
[CLI Usage](#cli-usage) •
[API & WebSockets](#api--websockets) •
[Documentation](#documentation) •
[Disclaimer](#disclaimer)

---

</div>

<a id="about"></a>
## About the Project

**SOLE (Secure Open Ledger for Education)** is a lightweight, UTXO-based distributed ledger developed as an educational and experimental blockchain platform at the **University of Salento (Università del Salento)**.

Rather than relying on resource-intensive Proof-of-Work, SOLE uses a deterministic **Proof-of-Authority (PoA)** consensus mechanism where authorized institutional keys validate and sign blocks every 10 seconds. The project combines a Bitcoin-style UTXO accounting model with a modern Go software stack, including BadgerDB v3 for persistent storage and libp2p for peer-to-peer communication.

---

<a id="features"></a>
<a id="key-highlights"></a>
## Features

- **Proof-of-Authority Consensus:** Blocks are forged at fixed 10-second intervals by pre-authorized validator keys, avoiding unnecessary computation while ensuring predictable block times.
- **UTXO Model & ECDSA Signatures:** Built on unspent transaction outputs with ECDSA cryptography on the NIST P-256 (`secp256r1`) curve and Base58Check address formatting.
- **BadgerDB v3 Storage:** Pure-Go key-value LSM engine with dedicated prefix indexes, enabling $O(1)$ balance queries and fast UTXO lookups.
- **Decoupled Light Client:** Routine CLI operations (creating wallets, checking balances, signing and sending transactions) talk to a running node over HTTP REST, so you do not need to lock or download the chain database locally.
- **Mempool Double-Spend Protection:** Pending transactions track spent outputs in memory to reject conflicting double-spend attempts before blocks are forged.
- **libp2p P2P Networking:** Built-in NAT traversal, local mDNS discovery, peer routing, and bounded concurrency with stream timeouts and mempool byte quotas.
- **OP_RETURN Data Notarization:** Transactions can carry up to 80 bytes of arbitrary metadata in an unspendable output, useful for timestamping hashes, academic attestations, or document signatures.

---

<a id="specifications"></a>
<a id="key-specifications"></a>
## Specifications

| Parameter | Value | Notes |
| :--- | :--- | :--- |
| **Consensus** | Proof-of-Authority (PoA) | Authorized validator keys sign block headers |
| **Accounting Model** | UTXO | Bitcoin-style inputs and outputs |
| **Block Time** | 10 seconds | Fixed block interval |
| **Storage Engine** | BadgerDB v3 | LSM-Tree key-value database |
| **Base Unit** | Photon ($10^{-8}$ SOLE) | $1\text{ SOLE} = 100,000,000\text{ Photons}$ |
| **Max Supply** | 8,910,000 SOLE | Hard cap across all halving periods |
| **Genesis Allocation** | 5,000,000 SOLE | Allocated in block #0 for network distribution |
| **Block Reward** | 10 SOLE | Initial block subsidy |
| **Halving Interval** | Every 195,500 blocks | Reward halves approximately every 22.6 months |
| **Cryptography** | NIST P-256, SHA-256, RIPEMD-160 | Standard ECDSA keypairs & BIP-39 12-word mnemonics |
| **Address Format** | Base58Check | Standard versioned address with checksum |
| **Metadata Limit** | 80 bytes | Embedded via zero-value `OP_RETURN` |
| **Standard Ports** | P2P: `3000` / API & WS: `8080` | Configurable via CLI flags or `config.yaml` |

---

<a id="architecture"></a>
<a id="repository-architecture"></a>
## Architecture

The project follows standard Go package conventions, separating executable entrypoints (`cmd/`) from internal reusable packages (`pkg/`):

```text
sole/
├── cmd/
│   └── sole-cli/                   # Command-line interface entrypoint
│       ├── main.go                 # Process startup
│       └── cli.go                  # Cobra commands (wallet, chain, node, tx)
├── pkg/
│   ├── api/                        # HTTP REST server and WebSocket streams
│   │   ├── api_server.go           # Endpoints and request handlers
│   │   ├── api_middleware.go       # Token-bucket rate limiting and CORS
│   │   └── websocket.go            # Live event hubs (/ws/blocks, /ws/mempool)
│   ├── consensus/                  # Proof-of-Authority validation logic
│   │   └── consensus.go            # Block signing, validator checks, threshold logic
│   ├── core/                       # Core blockchain primitives
│   │   ├── block.go                # Block header, hashing, and serialization
│   │   ├── transaction.go          # Transaction inputs, outputs, and verification
│   │   ├── merkle_tree.go          # Merkle tree calculation
│   │   ├── genesis.go              # Genesis block constants
│   │   ├── base58.go               # Base58 and Base58Check encoding
│   │   └── utils.go                # Cryptographic helper functions
│   ├── p2p/                        # libp2p networking subsystem
│   │   └── network.go              # Host lifecycle, peer exchange, and mempool sync
│   ├── storage/                    # Persistence layer with BadgerDB
│   │   ├── blockchain.go           # Block storage, iterators, and chain tip management
│   │   └── utxo_set.go             # Indexed UTXO cache with atomic batch updates
│   └── wallet/                     # Key management
│       ├── wallet.go               # BIP-39 mnemonic generation & key derivation
│       └── wallets.go              # Keystore file storage with encryption
├── docs/                           # Documentation
│   ├── api_reference.md            # REST API schemas and parameters
│   ├── cli_manual.md               # Complete CLI reference
│   └── whitepaper.md               # Technical whitepaper and design rationale
├── config.example.yaml             # Example configuration file
├── go.mod                          # Go module dependencies
└── LICENSE                         # MIT License
```

---

<a id="quickstart"></a>
<a id="quickstart--developer-experience"></a>
## Quickstart

### Prerequisites
- **Go**: Version `1.22` or higher
- **Git**

### 1. Build from Source

```bash
git clone https://github.com/nicolocarcagni/sole.git
cd sole

go build -o sole-cli ./cmd/sole-cli
./sole-cli --help
```

### 2. Initialize the Genesis Block

Before running a node, initialize the local database with the genesis block:

```bash
./sole-cli chain init
```

This creates the `./data/blocks` directory with the initial chain state.

### 3. Configuration (Optional)

You can run with defaults or customize settings in a `config.yaml` file:

```bash
cp config.example.yaml config.yaml
```

Sample configuration:

```yaml
node:
  port: 3000
  listen: "0.0.0.0"
  miner: "" # Set your address here only if you are an authorized validator

network:
  bootnodes: "/dns4/sole.nicolocarcagni.dev/tcp/3000/p2p/12D3KooWEtsfPSAJjJMueguEWXkK35PmyBSyiUvKCGsAEHPGXFSG"
  public_ip: ""
  public_dns: ""

api:
  port: 8080
  listen: "0.0.0.0"
```

Configuration precedence is: **CLI flags > config.yaml > built-in defaults**.

### 4. Start the Node

```bash
./sole-cli node start
```

On first launch, the node generates a local identity key (`node_key.dat`), attempts to connect to configured bootnodes, and starts listening for API requests on port `8080`.

---

<a id="cli-usage"></a>
<a id="essential-cli-cheat-sheet"></a>
## CLI Usage

The `sole-cli` executable operates in two modes:
- **Full Node:** `sole-cli node start` joins the P2P network and maintains the local database.
- **Light Client:** All other commands query the local or remote node over the REST API without locking local storage.

### Wallets (`wallet`)

```bash
# Create a new wallet and generate a 12-word recovery phrase
./sole-cli wallet create

# List all stored wallet addresses
./sole-cli wallet list

# Check the balance of a specific address
./sole-cli wallet balance --address <SOLE_ADDRESS>

# Recover an existing wallet using a 12-word mnemonic
./sole-cli wallet recover

# Export a private key
./sole-cli wallet export --address <SOLE_ADDRESS>

# Import an existing private key (hex)
./sole-cli wallet import --key <HEX_PRIVATE_KEY>
```

### Transactions (`tx`)

Transactions are created and signed locally; your private key is never transmitted over the network.

```bash
# Send funds to an address
./sole-cli tx send \
  --from <SENDER_ADDRESS> \
  --to <RECIPIENT_ADDRESS> \
  --amount 10.0 \
  --fee 0.001

# Send a transaction with an OP_RETURN memo (max 80 bytes)
./sole-cli tx send \
  --from <SENDER_ADDRESS> \
  --to <RECIPIENT_ADDRESS> \
  --amount 1.0 \
  --fee 0.001 \
  --memo "Academic attestation hash: 7a8f..."

# Preview transaction hex without broadcasting (dry run)
./sole-cli tx send \
  --from <SENDER_ADDRESS> \
  --to <RECIPIENT_ADDRESS> \
  --amount 1.0 \
  --dry-run
```

### Chain Management (`chain`)

```bash
# Print the block history from the current tip down to genesis
./sole-cli chain print

# Rebuild the local UTXO index
./sole-cli chain reindex

# Delete and reset local blockchain data
./sole-cli chain reset
```

---

<a id="api--websockets"></a>
<a id="rest-api--websocket-streams"></a>
## REST API & WebSockets

When a node is running, it exposes a REST API and WebSocket streams on port `8080` by default.

### Endpoints

| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `GET` | `/blocks/tip` | Current chain tip (height and block hash) |
| `GET` | `/blocks/{hash}` | Full block information, transactions, and validator signature |
| `GET` | `/balance/{address}` | Address balance in Photons |
| `GET` | `/utxos/{address}` | Spendable outputs for an address (excluding pending mempool spends) |
| `GET` | `/transaction/{id}` | Transaction details, inputs, outputs, and decoded memos |
| `GET` | `/network/peers` | Connected libp2p peers and multiaddresses |
| `GET` | `/consensus/validators` | List of authorized PoA validator public keys |
| `POST` | `/tx/send` | Submit a signed raw transaction hex to the mempool |

### WebSocket Streams

For real-time updates without polling:

- **`/ws/mempool`**: Emits new transactions as soon as they pass validation and enter the memory pool.
- **`/ws/blocks`**: Emits new blocks immediately after validator signing and local commit.

Example connection:

```javascript
const ws = new WebSocket('ws://localhost:8080/ws/blocks');

ws.onmessage = (event) => {
  const block = JSON.parse(event.data);
  console.log(`New block: #${block.data.height} (${block.data.hash})`);
};
```

---

<a id="documentation"></a>
## Documentation

Additional guides and technical details are in the [`/docs`](docs/) directory:

- [CLI Manual](docs/cli_manual.md): Flag details, light client configuration, and systemd service setup.
- [API Reference](docs/api_reference.md): Endpoint parameters, rate limits, and JSON schemas.
- [Technical Whitepaper](docs/whitepaper.md): Cryptographic choices, consensus model, and storage design.

---

<a id="testing"></a>
## Testing

Run tests across all packages:

```bash
# Run unit tests
go test -v ./...

# Run tests with the Go race detector
go test -race ./...

# Check test coverage
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

---

<a id="disclaimer"></a>
<a id="academic-disclaimer"></a>
## Disclaimer

> **Note on Research Software:**  
> SOLE is developed for academic instruction and distributed systems research at the University of Salento. While it implements standard cryptographic primitives (ECDSA on NIST P-256, SHA-256, BadgerDB, and libp2p), it **has not undergone an independent commercial security audit**. It is not designed or intended to hold real-world financial assets.

---

<a id="license"></a>
## License

This project is licensed under the terms of the [MIT License](LICENSE).
