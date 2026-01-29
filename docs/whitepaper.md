# SOLE Blockchain Technical Whitepaper v1.0
**Secure Open Ledger for Education**

**Authors:** Nicolò Carcagni (Dipartimento di Ingegneria dell'Informazione, Università del Salento)  
**Date:** January 2026  
**Status:** Working Draft (v1.0)

---

## 1. Abstract

**SOLE** is a lightweight, permissioned blockchain infrastructure designed for educational and research purposes at the University of Salento. Built from scratch in **Go**, it implements a hybrid architecture combining a **Proof of Authority (PoA)** consensus mechanism for efficiency with a robust **Libp2p** networking stack for distributed communication. Unlike computational-heavy Proof of Work systems, SOLE provides a sustainable, high-throughput environment suitable for academic experimentation, while maintaining core blockchain principles such as UTXO-based transactions, cryptographic verification, and decentralized P2P discovery.

## 2. Introduction

### The Problem
Traditional blockchain architectures like Bitcoin (Proof of Work) are often too resource-intensive and complex for effective usage in educational environments. Students and researchers require a system that is:
1.  **Efficient**: Capable of running on standard hardware (maybe Raspberry Pi) without mining farms.
2.  **Modular**: easy to extend for experiments (e.g., changing consensus, networking).

### The SOLE Solution
SOLE addresses these needs by stripping down the blockchain to its essentials while upgrading the technology stack:
*   **Language**: Written in Go for performance and compatibility.
*   **Storage**: Uses **BadgerDB**, a modern Key-Value store.
*   **Networking**: Leverages **Libp2p** (the same stack used by Ethereum 2.0) for peer connectivity.

## 3. System Architecture (The Core)

The core architecture is built around a linear chain of blocks secured by cryptographic hashes.

### Data Structures
The fundamental unit is the **Block**, defined as:

```go
type Block struct {
    Timestamp     int64
    Transactions  []*Transaction
    PrevBlockHash []byte
    Hash          []byte
    Height        int
    Validator     []byte // Public Key of the signer
    Signature     []byte // ECDSA Signature of the block hash
}
```

*   **Hash Linking**: Each block contains the SHA-256 hash of the previous block, creating an immutable chain.
*   **Merkle Strategy**: In progress.

### Persistence Layer
Data persistence is handled by **BadgerDB v3**, an embeddable, persistent, simple, and fast key-value (KV) store written in pure Go.
*   **Log-Structured Merge (LSM) Trees**: BadgerDB is optimized for write-heavy workloads, making it ideal for appending new blocks.
*   **Safety**: Automatic value verification (Checksums) and conflict detection are enabled.

### Serialization
To ensure efficient internal storage and network transmission, SOLE uses Go's `encoding/gob` for high-performance binary serialization. For external interaction (API/CLI), objects are marshaled to JSON.

## 4. Transaction Model (UTXO)

SOLE adopts the **Unspent Transaction Output (UTXO)** model, similar to Bitcoin, rather than the Account/Balance model of Ethereum. This provides better privacy and scalability potential.

### Logic
There is no "account balance" stored in the database. A user's balance is calculated by scanning the set of all **Unspent Outputs** locked to their public key.

### Inputs & Outputs
*   **TxOutput**: Represents "coins" existing in the system. It contains a `Value` (amount) and a `PubKeyHash` (locking script).
*   **TxInput**: Represents a reference to a previous output being spent. It includes:
    *   `Txid`: ID of the transaction created the output.
    *   `Vout`: Index of the output in that transaction.
    *   `Signature`: Proof of ownership (ECDSA).
    *   `PubKey`: Public key to verify the signature.

### Flow of Funds
When a transaction is created:
1.  **Selection**: The node selects enough unspent outputs (UTXOs) to cover the transaction `Amount`.
2.  **Spending**: These UTXOs are referenced as Inputs.
3.  **Creation**: Two new outputs are typically created:
    *   **Recipient Output**: The amount sent to the receiver.
    *   **Change Output**: The remainder returned to the sender.

### Replay Protection
To prevent "Replay Attacks" (where a valid transaction is intercepted and re-broadcasted maliciously), every transaction struct includes a `Timestamp` (int64). This ensures that two identical transactions sent at different times will have different Hashes (IDs), preventing the network from treating them as duplicates.

## 5. Cryptography & Security

SOLE uses industry-standard cryptographic primitives to ensure security and identity.

*   **Hashing**: `SHA-256` for block hashes and Transaction IDs.
*   **Address Generation**: `RIPEMD-160` is applied to the SHA-256 hash of the Public Key to generate short, secure addresses.
*   **Digital Signatures**: `ECDSA` (Elliptic Curve Digital Signature Algorithm) over the **NIST P-256** curve is used for signing transactions and blocks.

### Identity
Addresses are formatted using **Base58Check** encoding, resulting in user-friendly strings starting with `1` (similar to Bitcoin Legacy addresses), which include a checksum to prevent typing errors.

## 6. Consensus Mechanism: Proof of Authority (PoA)

SOLE utilizes **Proof of Authority (PoA)**, a consensus mechanism where block creation is restricted to a set of pre-approved validators.

### Forging vs Mining
*   **No Mining**: There is no computationally expensive puzzle (Proof of Work) to solve. This drastically reduces energy consumption.
*   **Forging**: Authorized nodes ("Validators") take turns creating blocks.

### Validator Set
The list of authorized validators is currently managed via a hardcoded configuration in `consensus.go`. Validators are identified by their Public Keys.
*   **Rettore**
*   **Capo Dipartimento**
*   **Docenti**

### Block Verification
When a node receives a block, it verifies:
1.  **Signature**: The block must be signed by a valid ECDSA key.
2.  **Authority**: The signer's public key must exist in the `AuthorizedValidators` list.
3.  **Integrity**: The block hash must match the content.

## 7. Networking & P2P

The networking layer is built on top of **Libp2p**, a modular network stack.

### Discovery
*   **Local (mDNS)**: Nodes automatically discover peers on the same LAN (Local Area Network) using multicast DNS.
*   **Global (Bootstrap)**: For WAN (Internet) connectivity, nodes connect to hardcoded **Bootnodes** which act as entry points to the DHT (Distributed Hash Table).

### Protocol
Nodes communicate using a custom binary protocol over TCP/QUIC streams (`/sole/1.0.0`):
*   `Version`: Handshake message exchanging current block height.
*   `Inv`: "Inventory" message announcing new object hashes (Blocks/Txs).
*   `GetData`: Requesting full data for a specific hash.
*   `Block / Tx`: The actual data payload.

## 8. Ecosystem: Client & API

### REST API
The core node runs an HTTP API Server (default port `8080`) enabling external integration.
*   **Rate Limiting**: To prevent DoS attacks, the API implements Token Bucket rate limiting (middleware).
*   **Endpoints**: `/balance/{addr}`, `/tx/send`, `/blocks/tip`, `/transactions/{addr}`.

### Clients
*   **sole-cli**: The native Go command-line tool for node management and wallet operations.
*   **Python SDK**: An external ecosystem of Python scripts (Wallet, Telegram Bot) that interact with the node via the REST API, demonstrating the platform's interoperability.

## 9. Conclusion & Future Roadmap

The SOLE Blockchain successfully demonstrates a working, performant, and secure distributed ledger for educational use. By leveraging Go, BadgerDB, and Libp2p, it achieves a high level of technical maturity.

### Future Developments
1.  **Merkle Trees**: Implementing full Merkle Tree roots for efficient SPV (p-node) verification.
2.  **Dynamic Validator Set**: Moving the validator list from code to on-chain governance (voting).
