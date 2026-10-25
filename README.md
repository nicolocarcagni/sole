<div align="center">

# ☀️ SOLE Blockchain
### **Secure Open Ledger for Education**

*An educational, high-throughput, energy-efficient Proof-of-Authority (PoA) distributed ledger engineered in Go for the University of Salento.*

<p align="center">
  <a href="https://golang.org"><img src="https://img.shields.io/badge/Go-1.22%2B-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version" /></a>
  <a href="https://github.com/nicolocarcagni/sole/releases"><img src="https://img.shields.io/badge/Release-v4.0.0-blue?style=for-the-badge&logo=github" alt="Release v4.0.0" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-green.svg?style=for-the-badge" alt="License: MIT" /></a>
  <a href="https://www.unisalento.it"><img src="https://img.shields.io/badge/UniSalento-Academic%20Project-E30613?style=for-the-badge&logo=academia&logoColor=white" alt="UniSalento" /></a>
  <a href="https://github.com/nicolocarcagni/sole/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/nicolocarcagni/sole/ci.yml?branch=main&style=for-the-badge&logo=githubactions&logoColor=white&label=CI" alt="CI Status" /></a>
  <a href="https://github.com/nicolocarcagni/sole"><img src="https://img.shields.io/badge/Go%20Report-A%2B-brightgreen?style=for-the-badge&logo=go&logoColor=white" alt="Go Report Card" /></a>
</p>

<p align="center">
  <strong>Official Bootnode:</strong> <code>sole.nicolocarcagni.dev:3000</code> &nbsp;|&nbsp; 
  <strong>Network:</strong> <code>UniSalento PoA Testnet</code> &nbsp;|&nbsp; 
  <strong>Base Currency:</strong> <code>SOLE (10⁸ Photons)</code>
</p>

---

[Key Highlights](#key-highlights) •
[Specifications](#key-specifications) •
[Architecture](#repository-architecture) •
[Quickstart](#quickstart--developer-experience) •
[CLI Cheat-Sheet](#essential-cli-cheat-sheet) •
[API & WebSockets](#rest-api--websocket-streams) •
[Documentation](#documentation) •
[Disclaimer](#disclaimer)

---

</div>

## 📖 Executive Summary

**SOLE (Secure Open Ledger for Education)** is a modern, modular, UTXO-based distributed ledger developed as a pedagogical and experimental blockchain for the **University of Salento (Università del Salento)**.

Unlike legacy compute-intensive blockchains, SOLE implements an energy-efficient **Proof-of-Authority (PoA)** consensus mechanism tailored for institutional and campus environments. It bridges rigorous academic research with industrial-grade software engineering, providing zero-carbon finality, deterministic 10-second block intervals, and instant balance verification.

---

## ✨ Core Value Proposition & Key Highlights <a id="key-highlights"></a><a id="core-value-proposition--key-highlights"></a>

<div align="center">

| Feature | Description |
| :--- | :--- |
| ⚡ **Proof-of-Authority (PoA)** | Zero-energy consensus governed by trusted institutional validators (Faculty, Rectorate, Engineering Labs). Instant, deterministic 10s block forging. |
| 📦 **BadgerDB v3 LSM-Tree** | High-performance pure-Go key-value storage engine with prefix scanning. Achieves $O(1)$ UTXO balance checks and sub-millisecond retrieval. |
| 🔒 **Persistent UTXO Engine** | Bitcoin-inspired Unspent Transaction Output model with cryptographic ECDSA signatures on the NIST P-256 (`secp256r1`) curve and SHA-256/RIPEMD-160 hashing. |
| 🌐 **libp2p Hybrid Networking** | Enterprise peer-to-peer transport with NAT traversal, automatic mDNS peer discovery, DHT routing, and aggregate/per-peer stream quotas against DoS. |
| 📝 **80-Byte `OP_RETURN` Notarization** | Native zero-value provably unspendable outputs for on-chain credential notarization (CFU credits, exam certifications, diplomas) without state bloat. |
| 🤖 **AI Agent & Micro-Economy Ready** | Designed for programmatic micro-transactions, LLM agent simulation frameworks (`campus_sim.py`), and seamless custodial Telegram integrations. |

</div>

### Key Advantages:
- **Decoupled Light Client Mode:** The CLI interacts seamlessly with remote nodes over HTTP REST without needing to lock or download the multi-gigabyte blockchain ledger locally.
- **Mempool Double-Spend Shield:** In-memory mempool automatically tracks and isolates pending unspent outputs to prevent simultaneous double-spend submission.
- **Strict Byte & Stream Quotas:** Hardened against denial-of-service (DoS) via aggregate memory budgets (10 MiB mempool cap), per-peer quotas (2 MiB / 500 txs), and bounded stream worker pools (128 concurrent streams).
- **Zero Configuration Drift:** Unified configuration via `config.yaml` with strict precedence: `CLI Flags > config.yaml > Built-in Defaults`.

---

## 📊 Key Specifications <a id="key-specifications"></a><a id="specifications"></a>

| Parameter | Specification | Additional Details |
| :--- | :--- | :--- |
| **Consensus Mechanism** | **Proof-of-Authority (PoA)** | Authorized validator rotation via cryptographic block signatures |
| **Accounting Model** | **UTXO** | Unspent Transaction Output with deterministic input/output verification |
| **Storage Engine** | **BadgerDB v3** | Log-Structured Merge-tree (LSM) key-value store with value log |
| **Base Unit** | **Photon** ($10^{-8}$ SOLE) | $1\text{ SOLE} = 100,000,000\text{ Photons}$ |
| **Maximum Supply** | **8,910,000 SOLE** | Absolute mathematical hard cap enforced across all eras |
| **Genesis Allocation** | **5,000,000 SOLE** | Allocated to institutional admin wallet for student bootstrap funds |
| **Initial Block Subsidy** | **10 SOLE** | $1,000,000,000\text{ Photons}$ awarded per validated block |
| **Block Time Target** | **10 seconds** | Deterministic block interval |
| **Halving Interval** | **Every 195,500 blocks** | Block reward halves approximately every ~22.6 months |
| **Default P2P Port** | **`3000`** | libp2p wire protocol (`/sole/3.0.0`) |
| **Default RPC & WS Port** | **`8080`** | REST API & bi-directional WebSocket event hubs |
| **Cryptography Curve** | **NIST P-256 (secp256r1)** | Standardized ECDSA key pairs + BIP-39 12-word mnemonics |
| **Address Format** | **Base58Check** | Version byte + 20-byte PubKeyHash (SHA-256 + RIPEMD-160) + Checksum |
| **Metadata Capacity** | **80 bytes** | Zero-value `OP_RETURN` payload for immutable notarization |

---

## 🏗️ Repository Architecture <a id="repository-architecture"></a><a id="architecture"></a>

The project adheres to standard idiomatic Go package layout, separating operational entrypoints (`cmd/`) from modular internal domain logic (`pkg/`):

```text
sole/
├── cmd/
│   └── sole-cli/                   # Command-line binary entrypoint & routing
│       ├── main.go                 # Execution bootstrap
│       └── cli.go                  # Cobra & Viper commands (wallet, chain, node, tx)
├── pkg/
│   ├── api/                        # HTTP REST API & WebSocket gateways
│   │   ├── api_server.go           # REST routing, handlers, and JSON endpoints
│   │   ├── api_middleware.go       # Rate limiting (Token Bucket) & CORS headers
│   │   └── websocket.go            # Reactive event hubs (/ws/blocks, /ws/mempool)
│   ├── consensus/                  # Proof-of-Authority engine
│   │   ├── consensus.go            # Validator authorization, block signing & verification
│   │   └── consensus_test.go       # Cryptographic consensus unit tests
│   ├── core/                       # Blockchain protocol primitives
│   │   ├── block.go                # Block structure, serialization, and hashing
│   │   ├── transaction.go          # UTXO inputs/outputs, TX validation, OP_RETURN
│   │   ├── merkle_tree.go          # Cryptographic Merkle tree computation
│   │   ├── genesis.go              # Genesis block constants and inscription
│   │   ├── base58.go               # Base58 and Base58Check encoder/decoder
│   │   └── utils.go                # Cryptographic hashing & byte manipulation
│   ├── p2p/                        # libp2p networking subsystem
│   │   ├── network.go              # Host lifecycle, stream muxing, discovery & protocols
│   │   ├── network_test.go         # Stream concurrency, timeout & shutdown tests
│   │   └── mempool_test.go         # Mempool byte quotas and DoS defense test suites
│   ├── storage/                    # Persistence layer (BadgerDB v3)
│   │   ├── blockchain.go           # Blockchain state, block iterator, chain tip logic
│   │   ├── utxo_set.go             # Indexed unspent output cache with atomic updates
│   │   ├── fork_choice_test.go     # Chain re-organization & fork choice tests
│   │   └── stability_test.go       # Storage crash recovery and concurrency tests
│   └── wallet/                     # Key management & cryptography
│       ├── wallet.go               # BIP-39 mnemonic derivation, ECDSA key generation
│       ├── wallets.go              # Encrypted keystore persistence & Argon2id/AES
│       └── wallets_test.go         # Wallet cryptographic derivation tests
├── docs/                           # Comprehensive technical specifications
│   ├── api_reference.md            # HTTP endpoints and JSON schemas
│   ├── cli_manual.md               # Full command-line interface documentation
│   └── whitepaper.md               # Consensus mathematics & economic whitepaper
├── config.example.yaml             # Canonical configuration template
├── go.mod                          # Dependency definitions (Go 1.22+)
└── LICENSE                         # MIT License
```

---

## 🚀 Quickstart & Developer Experience <a id="quickstart--developer-experience"></a><a id="quickstart"></a>

### Prerequisites
- **Go**: Version `1.22` or higher (verified up to Go `1.25.x`)
- **Git**: Latest release
- **Operating System**: Linux, macOS, or Windows (WSL2 recommended)

### 1. Clone & Build

```bash
# Clone the repository
git clone https://github.com/nicolocarcagni/sole.git
cd sole

# Verify dependencies and compile sole-cli
go mod verify
go build -o sole-cli ./cmd/sole-cli

# Verify successful installation
./sole-cli --help
```

### 2. Initialize Blockchain State

Every node requires initializing the local storage layer and Genesis block:

```bash
# Bootstrap local BadgerDB state with the official Genesis block
./sole-cli chain init
```

> [!NOTE]
> This command creates the `./data/blocks` directory with the immutable UniSalento Genesis block (Block #0).

### 3. Configure the Node (Optional)

Create your local runtime configuration:

```bash
cp config.example.yaml config.yaml
```

Inspect or customize your `config.yaml`:
```yaml
node:
  port: 3000
  listen: "0.0.0.0"
  miner: "" # Insert your validator address if authorized to forge blocks

network:
  # Official UniSalento Seed Bootnode
  bootnodes: "/dns4/sole.nicolocarcagni.dev/tcp/3000/p2p/12D3KooWEtsfPSAJjJMueguEWXkK35PmyBSyiUvKCGsAEHPGXFSG"
  public_ip: ""
  public_dns: ""

api:
  port: 8080
  listen: "0.0.0.0"
```

### 4. Run the Node

Start your node to connect to the peer-to-peer network and expose the REST/WebSocket API:

```bash
# Start the full node with automatic discovery
./sole-cli node start
```

On first startup, the node automatically provisions its persistent libp2p cryptographic identity (`node_key.dat`), attempts connection to the seed bootnode, activates mDNS local discovery, and starts the API listener on `http://localhost:8080`.

---

## ⚡ Essential CLI Cheat-Sheet <a id="essential-cli-cheat-sheet"></a><a id="cli-cheat-sheet"></a>

The `sole-cli` functions as both a **Full Node daemon** and an ultra-fast **Light Client**. When running wallet or query commands, it operates in light-client mode by interacting with the local or remote API without locking the database.

### 🔑 Wallet Operations (`wallet`)

```bash
# 1. Generate a new wallet with a secure 12-word BIP-39 mnemonic
./sole-cli wallet create

# 2. List all local addresses and their public keys
./sole-cli wallet list

# 3. Check Photon & SOLE balance for an address
./sole-cli wallet balance --address <SOLE_ADDRESS>

# 4. Recover a wallet using your 12-word recovery seed phrase
./sole-cli wallet recover

# 5. Export private key in hexadecimal format
./sole-cli wallet export --address <SOLE_ADDRESS>

# 6. Import an existing raw private key
./sole-cli wallet import --key <HEX_PRIVATE_KEY>
```

### 💸 Transaction Operations (`tx`)

Transactions are constructed and signed locally on your machine—your private keys never leave the client.

```bash
# Standard transaction transfer
./sole-cli tx send \
  --from <SENDER_ADDRESS> \
  --to <RECIPIENT_ADDRESS> \
  --amount 25.5 \
  --fee 0.001

# Transaction with an 80-byte notarization memo (OP_RETURN)
./sole-cli tx send \
  --from <SENDER_ADDRESS> \
  --to <RECIPIENT_ADDRESS> \
  --amount 1.0 \
  --fee 0.001 \
  --memo "CFU-CERT:INF-01:30L:UNISALENTO"

# Dry run (generate and display raw signed hex without broadcasting)
./sole-cli tx send \
  --from <SENDER_ADDRESS> \
  --to <RECIPIENT_ADDRESS> \
  --amount 10.0 \
  --dry-run
```

### ⛓️ Chain Maintenance (`chain`)

```bash
# Print complete topological block history from tip to genesis
./sole-cli chain print

# Recompute and rebuild the persistent UTXO database index
./sole-cli chain reindex

# Reset and purge local blockchain database (Requires caution!)
./sole-cli chain reset
```


---

## 🔌 REST API & WebSocket Streams <a id="rest-api--websocket-streams"></a><a id="api"></a>

The node runs an embedded high-performance REST API and WebSocket daemon on port `8080`.

### Core HTTP Endpoints

| Method | Endpoint | Description | Rate Limit |
| :--- | :--- | :--- | :--- |
| `GET` | `/blocks/tip` | Fetch current highest block height and hash | 20 req/s |
| `GET` | `/blocks/{hash}` | Query full block details, transactions, and validator signatures | 20 req/s |
| `GET` | `/balance/{address}` | Instant $O(1)$ balance lookup in Photons | 20 req/s |
| `GET` | `/utxos/{address}` | Fetch spendable unspent outputs (mempool double-spend filtered) | 20 req/s |
| `GET` | `/transaction/{id}` | Detailed transaction data including decoded `OP_RETURN` memos | 20 req/s |
| `GET` | `/network/peers` | Connected libp2p peers and multiaddresses | 20 req/s |
| `GET` | `/consensus/validators`| List of authorized PoA validator public keys | 20 req/s |
| `POST` | `/tx/send` | Broadcast a cryptographically signed raw transaction hex | 5 req/s |

### Real-time Reactive Streams (WebSockets)

Bi-directional WebSocket streams empower frontends, explorers, and monitoring dashboards to react instantly to network state changes:

- **`/ws/mempool`**: Broadcasts every incoming transaction the instant it passes validation and enters the memory pool.
- **`/ws/blocks`**: Emits real-time block notifications immediately upon validator signing and ledger commitment.

```javascript
// Example: Connect to live block events
const socket = new WebSocket('ws://localhost:8080/ws/blocks');

socket.onmessage = (event) => {
  const block = JSON.parse(event.data);
  console.log(`⚡ New Block Forged: #${block.data.height} [${block.data.hash}]`);
};
```

---

## 📚 Documentation <a id="documentation"></a>

Detailed technical references and guides are maintained in the [`/docs`](docs/) directory:

- 📘 **[CLI Manual](docs/cli_manual.md)**: Exhaustive manual for `sole-cli`, command syntax, flags, and systemd service configurations.
- 🌐 **[API Reference](docs/api_reference.md)**: Complete OpenAPI/REST endpoint schemas, request/response payloads, and error codes.
- 🔬 **[Technical Whitepaper](docs/whitepaper.md)**: In-depth analysis of the Proof-of-Authority consensus, economic halving curves, BadgerDB LSM indexing, and libp2p topology.

---

## 🧪 Testing & Quality Assurance

SOLE is tested against concurrency races, network stress, and malformed inputs:

```bash
# Run unit tests across all packages
go test -v ./...

# Run test suite with the Go race detector enabled
go test -race ./...

# Generate test coverage report
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

---

<a id="disclaimer"></a>
<a id="academic-disclaimer"></a>
## 🛡️ Academic Disclaimer

> [!CAUTION]
> **Educational & Research Software Only**  
> SOLE Blockchain is developed strictly for educational instruction, computer science research, and institutional prototyping at the University of Salento. While the project utilizes industry-standard cryptographic algorithms (NIST P-256 ECDSA, SHA-256, BadgerDB v3, and libp2p), it **has not undergone an independent commercial security audit**. 
> 
> Under no circumstances should SOLE tokens or network infrastructure be utilized for speculative financial investment, commercial trading, or real-world monetary value storage.

---

## 📄 License

This project is licensed under the terms of the [MIT License](LICENSE).
