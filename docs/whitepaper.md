# SOLE Blockchain: A Technical Deep-Dive (v3.0.0)
**The Ledger for the University of Salento**

**By:** Nicolò Carcagni (Dept. of Engineering, Unisalento)  
**Updated:** March 2026

---

## 1. What is SOLE?

We built SOLE to give our university a fast, ownable way to move value. It’s not just code; it’s a tool for students and researchers. Whether you're sending a few SOLE to a friend for a coffee at the faculty bar or tracking research credits, the v3.0.0 "Hard Fork" makes the network faster and much easier to use. 

We’ve swapped out slow proof-of-work for a Proof of Authority (PoA) system. We also added BIP39 mnemonics so you can stop wrestling with raw hex keys.

## 2. Under the Hood

We strictly use Go. It’s fast, handles concurrency like a pro, and has great libraries for networking.

### How we store data
We use **BadgerDB v3** for persistent storage. It’s a key-value store based on LSM trees. We’ve indexed every transaction ID to its block hash, and every unspent output to its owner. This means checking a balance or finding a transaction is an instant prefix scan—$O(1)$ complexity—instead of a slow, multi-second chain crawl.

### The Light Client Model
In v3.0.0, we decoupled the CLI from the Node. The CLI now acts as a "Light Client." It doesn't need to lock or copy the 2GB blockchain database just to send a payment. Instead, it asks the Node for unspent outputs via a REST API, builds the transaction, and signs it. This makes the wallet fast enough to run on a laptop without a specialized setup.

### Moving data around
We use `encoding/gob` for speed when nodes talk peer-to-peer. For the outside world—like our mobile wallets and WebSockets—we use standard JSON over a REST API.

## 3. Security (Without the Headaches)

We want SOLE to be secure but practical.

*   **12-Word Seeds**: You don't need to copy-pasting hex strings. We use BIP39 seed phrases. 12 words give you 128 bits of entropy. Type those 12 words into any SOLE-compatible wallet and your funds are back.
*   **Local Signing**: Your private keys stay on your machine. When you send SOLE, the CLI signs the transaction locally using your key. The Node only ever sees the final, signed transaction hex. Your secrets never travel over the network.
*   **Signatures**: We use ECDSA on the **NIST P-256** (secp256r1) curve. It’s standard, fast, and secure.
*   **Clean Addresses**: We hash your public key with SHA-256 and RIPEMD-160, then wrap it in Base58Check. This gives you a clean address that’s easy to share.

## 4. The Transaction Model (UTXO)

SOLE doesn't use account balances like a bank. We use the **UTXO model**, just like Bitcoin. Your "balance" is actually a collection of unspent outputs waiting for you to unlock them.

### Keeping the Mempool Clean
Double-spending is the enemy. Our v3.0.0 API is mempool-aware. If you send a transaction, the Node’s API immediately knows those coins are "pending." If you try to build another transaction before the first one is mined, the API filters out the pending coins, preventing you from accidentally creating a conflict. We vet everything before it even hits a block.

### Memos (OP_RETURN)
You can attach an 80-byte memo to your transaction. It’s recorded on the chain forever but doesn't slow down the node's memory.

## 5. Consensus: Our Proof of Authority

We don't waste electricity mining. Instead, we trust trusted identities. 

### Who forges blocks?
Only authorized validators—like the **Department of Engineering**, the **Rectorate**, or specific campus labs—can add blocks to the chain. They forge a new block exactly every **10 seconds**. Every block header is signed by a validator's key, so the network knows exactly who to trust.

## 6. Tokenomics: The Unisalento Model

We wanted an economic model that feels alive during a semester.

*   **Units**: 1 SOLE equals $10^8$ Photons. 
*   **Genesis**: We started with 5,000,000 SOLE in the admin wallet to bootstrap the ecosystem.
*   **The Reward**: Validators get 10 SOLE for every block they forge.
*   **Halving**: The reward drops by half every 195,500 blocks. 
*   **Hard Cap**: We will never have more than **8,910,000 SOLE**.

## 7. Networking & Real-time Events

### LibP2P
We use **Libp2p** for p2p communication (protocol `/sole/3.0.0`). We added mutex guards to the peer maps and mempools Since the CLI is now a light client, it only connects to the P2P network if you are actually running it as a node.

### Live Streams
If you’re building an app for SOLE, you don't need to poll the API. You can hook into our WebSocket hubs:
*   `/ws/mempool`: Get a ping the second a new transaction hits the network.
*   `/ws/blocks`: Get notified as soon as a block is forged.

## 8. Join the Ecosystem

*   **SOLE CLI**: The Swiss Army knife for the network. It now works in two modes: a high-performance **Full Node** for validators, or a **Light Client** for everyday wallet management.
*   **[Swallet](https://github.com/nicolocarcagni/Swallet)**: The desktop and mobile app. It talks to the REST API and handles the BIP39 complexity for you.
*   **REST API**: The backbone of the ecosystem. Every action—from checking a balance to pushing a signed transaction—happens here.