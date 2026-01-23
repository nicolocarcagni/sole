# Getting Started with SOLE (Zero to Hero)

Welcome to the **SOLE** (Salento On-Line Economy) project! This guide will take you from zero to running a full node on the Unisalento network.

## Prerequisites
*   **Go** (Golang) Version 1.22+ installed.
*   Terminal / Command Prompt access.
*   **Git** installed.

---

## Step 1: Download and Build
Clone the official repository and compile the source code.

```bash
# 1. Clone the repo
git clone https://github.com/nicolocarcagni/sole.git
cd sole

# 2. Build the binary
go build -o sole-cli .

# 3. Verify installation
./sole-cli --help
```
*If you see the help menu, you are ready!*

---

## Step 2: Initialize the Blockchain
Before running a node, you need to initialize the local database with the Genesis Block.

```bash
./sole-cli init
```
*Output:*
> ☀️ SOLE Blockchain Inizializzata!
> - Genesis Hash: ...
> - Network: Unisalento Mainnet

---

## Step 3: Create Your Identity (Wallet)
You need a wallet to send and receive tokens.

```bash
# Generate a new wallet
./sole-cli createwallet
```
Write down safe the address generated (e.g., `1ExAmpLe...`). This is your public identity.

---

## Step 4: Join the Network
Start your node to connect with other peers at the University.

```bash
# Start on port 3000
./sole-cli startnode --port 3000
```
Your node will now:
1.  Discover peers on the local network.
2.  Download the latest blocks (Synchronize).
3.  Listen for new transactions.

**Keep this terminal window open.**

---

## Step 5: Receive Tokens
Since SOLE uses Proof of Authority, new tokens are not mined by computation but distributed by the Admin.
1.  Share your address from **Step 3** with the Network Administrator.
2.  Wait for them to send you a transaction.
3.  Check your balance (open a new terminal window):

```bash
./sole-cli getbalance --address <YOUR_ADDRESS>
```

---

## Step 6: Become a Validator (Advanced)
If you are authorized to secure the network:
1.  Get your Private Key: `./sole-cli printwallet --address <ADDR>`
2.  Register your key with the Governance team (updates `consensus.go`).
3.  Start your node in mining mode:
    ```bash
    ./sole-cli startnode --miner <YOUR_ADDRESS>
    ```
