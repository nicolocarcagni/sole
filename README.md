# SOLE Blockchain

![Go Report](https://goreportcard.com/badge/github.com/nicolocarcagni/sole)
![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Status](https://img.shields.io/badge/status-active-success.svg)

> **SOLE** is an academic **Proof-of-Authority (PoA)** blockchain implementation written in Go.  
It powers the **Unisalento** digital token ecosystem, designed as a lightweight, performant distributed ledger for educational and research purposes.

---

## 🏛 Project Overview

The SOLE network is designed to be a **Hybrid P2P Network** that bridges the gap between robust, always-online public nodes and ephemeral private clients.

*   **Consensus**: Proof of Authority (PoA). Selected validators (Rettore, Capo dipartimento, Docenti) sign blocks, ensuring low energy consumption and high throughput.
*   **Performance**: **Persistent UTXO Set** (BadgerDB backed) for O(1) balance checks and instant transactions.
*   **Tokenomics**: **Max Supply 19.55M SOLE**, Halving every 195.5k blocks, designed for long-term sustainability.
*   **Networking**: Built on `libp2p`. Supports DHT Discovery, NAT Traversal, and MDNS for local peers.
*   **Storage**: Uses BadgerDB (v3), a fast key-value store optimized for SSDs.
*   **Interoperability**: Exposes a RESTful JSON API with **CORS Support** and **Rich JSON Responses** (Sender/Receiver Address resolution) for easy integration with Web Explorers and Wallets.

## 🚀 Getting Started

### Prerequisites
*   **Go** 1.19 or higher

### Installation

Clone the repository and build the CLI tool:

```bash
git clone https://github.com/nicolocarcagni/sole.git
cd sole
go build -o sole-cli .
```

### Running a Node

To join the main network immediately (Zero-Config):

```bash
./sole-cli init
./sole-cli node start
```

The node will automatically:
1.  Initialize a secure identity (`node_key.dat`).
2.  Connect to the default public bootnodes (`sole.nicolocarcagni.dev`).
3.  Begin synchronizing the blockchain.

---

## 🛠 Command Line Interface

The `sole-cli` tool manages all aspects of the node and wallet.

### Wallet Management

```bash
# Create a new wallet address and keypair
./sole-cli wallet create

# Check the balance of an address
./sole-cli getbalance --address <ADDRESS>
```

### Transactions

Send tokens (SOLE) to another address.

```bash
./sole-cli send --from <SENDER> --to <RECEIVER> --amount <VALUE>
```

### Mining (Validators Only)

If you hold a validator key, you can start the node in mining mode:

```bash
./sole-cli node start --miner <VALIDATOR_ADDRESS>
```

---

## 🔌 API Integration

Developers can interact with the node via HTTP. The default port is `8080`.

| Endpoint | Method | Description |
| :--- | :--- | :--- |
| `/blocks/tip` | `GET` | Get current chain height and hash. |
| `/balance/{address}` | `GET` | Get confirmed balance. |
| `/transactions/{address}` | `GET` | Get full transaction history. |
| `/transaction/{id}` | `GET` | Get details of a single transaction. |
| `/tx/send` | `POST` | Broadcast a signed transaction. |

> See the full [API Reference](docs/api_reference.md) for details.

---

## 📚 Documentation

Detailed documentation is available in the `docs/` directory:

*   **[API Reference](docs/api_reference.md)**: complete endpoints specification.
*   **[CLI Manual](docs/cli_manual.md)**: flags and advanced configuration.

---

## Disclaimer

> This software is a **Proof of Concept (PoC)** developed for the **University of Salento**.  
It is intended (sole)ly for academic research and testing. It applies cryptographic primitives (ECDSA, SHA-256, RIPEMD160) but has not undergone a professional security audit. **Do not use for real-world financial assets.**
