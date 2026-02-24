# SOLE Blockchain Technical Whitepaper v1.0
**Secure Open Ledger for Education**

**Authors:** Nicolò Carcagni (Dipartimento di Ingegneria dell'Informazione, Università del Salento)  
**Date:** February 2026  
**Status:** Release 1.0

---

## 1. Abstract

SOLE is a lightweight, permissioned blockchain infrastructure designed for educational and research purposes at the University of Salento. Implemented in Go, it features a Hybrid Proof of Authority (PoA) consensus mechanism and a robust Libp2p networking stack for peer-to-peer distributed communication. The architecture utilizes persistent Unspent Transaction Output (UTXO) accounting and strict cryptographic guarantees to prevent double-spending and ensure immutable ledger integrity.

## 2. System Architecture

The SOLE core architecture is structured around a sequential, cryptographically linked chain of blocks.

### Persistence Layer
Data persistence is handled by **BadgerDB v3**, an embeddable, pure-Go Key-Value store based on Log-Structured Merge (LSM) Trees. The database structure includes parallel indices mapping transaction IDs (`tx-ID`) directly to block hashes, achieving $O(1)$ transaction lookup complexity. Overarching state integrity is guarded by value checksum verification and strict concurrent access locks.

### Transport and Storage Serialization
All internal storage operations and network peer synchronizations utilize `encoding/gob` for high-performance binary serialization. Externally exposed API endpoints execute deterministic JSON marshaling.

## 3. Cryptography & Security

SOLE employs standard cryptographic primitives to verify authority and maintain transaction immutability.

*   **Hashing Protocol**: `SHA-256` is uniformly enforced for block header calculation and Transaction ID generation.
*   **Signatures**: Digital signatures use the Elliptic Curve Digital Signature Algorithm (`ECDSA`) over the **NIST P-256** curve. The node natively parses standard uncompressed keys (65 bytes with `0x04` prefix) and raw structural coordinate vectors.
*   **Identity Extraction**: Public keys are hashed sequentially with `SHA-256` and `RIPEMD-160`. Base58Check encoding converts payloads into string addresses mapped to the version prefix `1`.

## 4. Transaction Model (UTXO)

The ledger operates entirely on the **Unspent Transaction Output (UTXO)** architecture. State vectors are exclusively defined by the set of all unspent outputs bound to specific public key hashes. 

### Intra-Block State Verification
The `UTXOSet` performs comprehensive validation covering multi-topology attack vectors. For any received transaction, the node verifies input existence within the BadgerDB persistent state. To mitigate mempool chain attacks, the protocol executes a two-pass `ValidateBlockTransactions` routine on newly minted blocks, actively tracking intra-block output lifecycles to unequivocally block simultaneous double-spends within identical block parameters.

### Replay Protection
All transactions instantiate an intrinsic UNIX Timestamp (`int64`), guaranteeing uniqueness for structurally identical spending patterns and rendering cross-chain or delayed replay broadcasting ineffective.

## 5. Merkle Tree Implementation

Integrity at the block level relies on a full **Binary Merkle Tree**. 
Transaction identifiers are hashed symmetrically to trace a single **Merkle Root**, integrated into the block header prior to consensus hashing. In scenarios involving odd-numbered transaction vectors, the final leaf node duplicates to preserve symmetric topology. The implementation inherently supports Simple Payment Verification (SPV) proofs ($O(\log N)$).

## 6. Consensus Mechanism: Hardened Proof of Authority (PoA)

Network consensus relies on a curated list of authorized public identities (Validators). Nodes cross-reference block signatures against an immutable hex-encoded whitelist (`consensus.go`).

### Cryptographic Rate Limiting
To mitigate identity-hijacking and Distributed Denial of Service (DDoS) exploitation, the PoA loop implements cryptographic spam prevention:
1.  **Symbolic Difficulty**: Forging block headers requires an incremental `Nonce` search enforcing a minimum target condition (e.g., `TargetZeros = 1` initiating bytes must resolve strictly to `0x00`). 
2.  **Temporal Drift**: Headers must satisfy strictly monotonic temporal rules and cannot exceed a predefined positive clock drift (`DriftTolerance = 1 * time.Minute`) against synchronizing peers.

### Strict Head Signature Binding
Validator ECDSA signatures are strictly mapped against the deterministic `SHA-256` header array (`PrevHash + MerkleRoot + Timestamp + Height + Nonce`). The signature payload is entirely isolated from the hash generation function, comprehensively resolving malleability vector vulnerabilities.

## 7. Economic Model (Tokenomics)

The economic issuance dynamically mimics deflationary issuance matrices.

*   **Precision Unit**: The fundamental ledger unit is the Photon (1 SOLE = $10^8$ Photons).
*   **Maximum Supply Cap**: Structurally restricted to 19,550,000 SOLE (honoring the 1955 founding year of Unisalento).
*   **Issuance Schedule**: Initial block reward dictates 50 SOLE per block, subjected to strict integer division (Halving) precisely every 195,500 blocks. After 64 cycles, emission halts entirely.

## 8. Network Operations

Distributed consensus operates over the **Libp2p** protocol stack (`/sole/1.0.0`). 
Local network traversal leverages `mDNS` multicasting for transparent LAN peering. Extranet operations synchronize through hardcoded WAN bootnodes establishing persistent `TCP/QUIC` socket tunnels through standard NAT barriers. Protocol state is handled via standardized network structures (`Version`, `Inv`, `GetData`, `Block`, `Tx`).
