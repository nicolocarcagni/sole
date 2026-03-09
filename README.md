---
![Go Report](https://goreportcard.com/badge/github.com/nicolocarcagni/sole)
![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Status](https://img.shields.io/badge/status-active-success.svg)
---

# SOLE Blockchain

SOLE (Secure Open Ledger for Education) is an academic Proof-of-Authority (PoA) blockchain implemented in Go. It provides a lightweight, performant distributed ledger environment designed specifically for educational integration and research at the University of Salento.

## Project Overview

The SOLE network operates as a Hybrid P2P Network, prioritizing high throughput and immediate finality without the energy overhead of computational mining.

*   **Consensus**: Proof of Authority (PoA). A deterministic set of authorized validators (e.g., University Administration, Faculty) securely signs blocks.
*   **Data Structure**: Features a Persistent UTXO Set backed by BadgerDB for strict double-spend prevention and $O(1)$ balance verification.
*   **Economic Market**: Utilizes implicit UTXO fees (`Sum(Inputs) - Sum(Outputs)`), descending mempool priority sorting, and dynamic miner rewards guarded by strict inflation resistance protocols.
*   **Mempool Integrity**: Features an autonomous 1-hour internal Time-To-Live (TTL) background garbage collector bypassing runtime timestamp spoofing attempts.
*   **On-Chain Metadata**: Natively supports appending 80-byte `OP_RETURN` textual payloads via unspendable 0-value outputs to transactions, explicitly bypassing cache bloat.
*   **Networking**: Operates on a modular `libp2p` stack supporting DHT-based peer discovery, NAT traversal, and automatic mDNS for LAN synchronization.

## Prerequisites

*   Go 1.19 or higher

## Quickstart

### Pre-Compiled Binaries

You can ownload them directly from the Releases page.

### Build the Node from Source

Clone the repository and compile the CLI executable manually:

```bash
git clone https://github.com/nicolocarcagni/sole.git
cd sole
go build -o sole-cli .
```

### Run the Node

To bootstrap the local persistence layer and immediately join the public P2P network:

```bash
./sole-cli chain init
./sole-cli node start
```

This sequence automatically generates a persistent P2P identity (`node_key.dat`), attempts to reach the configured bootstrap nodes, and begins block synchronization.

## Documentation

Comprehensive technical documentation is maintained in the `docs/` directory.

*   **[CLI Manual](docs/cli_manual.md)**: Detailed command-line usage and node configuration flags.
*   **[API Reference](docs/api_reference.md)**: Exact JSON schemas and REST endpoints for external integrators.
*   **[Technical Whitepaper](docs/whitepaper.md)**: Cryptographic specifications and consensus architecture.

## Disclaimer

This software is provided exclusively for academic research and testing. While it implements standard cryptographic primitives (ECDSA, NIST P-256, SHA-256), the codebase has not undergone a commercial security audit. It is strictly prohibited from holding real-world financial assets.
