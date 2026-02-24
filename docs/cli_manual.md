# SOLE CLI Manual

The `sole-cli` provides the primary interface for managing wallets, transacting funds, configuring the local database, and running a P2P node for the SOLE blockchain.

## Command Structure

The CLI is organized into four main categories: `wallet`, `chain`, `node`, and `tx`. 
Syntax: `./sole-cli <category> <command> [flags]`

---

## 1. Wallet Management (`wallet`)

Commands to create, import, export, and inspect cryptographic keys and balances.

### `create`
Generates a new ECDSA keypair and saves it to the local `wallet.dat` database.
*   **Flags:** None
*   **Example:**
    ```bash
    ./sole-cli wallet create
    ```

### `list`
Lists all public addresses stored in the local wallet file.
*   **Flags:** None
*   **Example:**
    ```bash
    ./sole-cli wallet list
    ```

### `import`
Imports an existing wallet using its raw private key in hexadecimal format.
*   **Required Flags:**
    *   `--key <HEX>`: The private key string (must be valid 64-character hex).
*   **Example:**
    ```bash
    ./sole-cli wallet import --key a1b2c3d4e5f6...
    ```

### `remove`
Deletes a specific wallet from the local file securely.
*   **Required Flags:**
    *   `--address <ADDR>`: The Base58-encoded address to remove.
*   **Example:**
    ```bash
    ./sole-cli wallet remove --address 1HSYNy8yXUuUZrkBCnzSc34Lqr8soPAKQL
    ```

### `balance`
Fetches the confirmed confirmed balance (available Unspent Transaction Outputs) for a given address.
*   **Required Flags:**
    *   `--address <ADDR>`: The Base58-encoded address.
*   **Example:**
    ```bash
    ./sole-cli wallet balance --address 1HSYNy8yXUuUZrkBCnzSc34Lqr8soPAKQL
    ```

### `export`
Prints the private key (in Hex) associated with an address for backup purposes.
*   **Required Flags:**
    *   `--address <ADDR>`: The Base58-encoded address.
*   **Example:**
    ```bash
    ./sole-cli wallet export --address 1HSYNy8yXUuUZrkBCnzSc34Lqr8soPAKQL
    ```

---

## 2. Blockchain Operations (`chain`)

Commands to bootstrap, verify, and maintain the persistent BadgerDB block store.

### `init`
Initializes a new contiguous blockchain database starting with the official Genesis Block. Must be run before starting a fresh node.
*   **Flags:** None
*   **Example:**
    ```bash
    ./sole-cli chain init
    ```

### `reindex`
Forces a complete rebuild of the UTXO (Unspent Transaction Output) cache from the main blockchain index. Useful for correcting local state cache corruptions.
*   **Flags:** None
*   **Example:**
    ```bash
    ./sole-cli chain reindex
    ```

### `print`
Iterates through the entire sequential ledger from the current Tip down to the Genesis block, outputting all transaction data to standard output.
*   **Flags:** None
*   **Example:**
    ```bash
    ./sole-cli chain print
    ```

### `reset`
Irreversibly deletes the current local blockchain database.
*   **Flags:** None
*   **Example:**
    ```bash
    ./sole-cli chain reset
    ```

---

## 3. Node & Network (`node`)

Commands to spin up the libp2p network interface, REST API server, and block forging procedures.

### `start`
Bootstraps the P2P networking stack and the local REST API server. Can optionally initiate the block forging engine if an authorized validator key is provided.
*   **Optional Flags:**
    *   `--port <INT>`: Local P2P TCP port (Default: 3000).
    *   `--listen <IP>`: Interface address to bind the P2P listener (Default: 0.0.0.0).
    *   `--public-ip <IP>`: Publicly routable IP address to announce to the DHT.
    *   `--public-dns <DOMAIN>`: Publicly routable DNS record to announce to the DHT.
    *   `--bootnodes <MULTIADDRS>`: Comma-separated libp2p multiaddrs to use for bootstrapping discovery.
    *   `--miner <ADDR>`: Base58-encoded address of an authorized validator to enable block forging.
    *   `--api-port <INT>`: Local REST API Server port (Default: 8080).
    *   `--api-listen <IP>`: Interface address to bind the REST API listener (Default: 0.0.0.0).
*   **Example:**
    ```bash
    ./sole-cli node start --port 3000 --miner 1HSYNy8yXUuUZrkBCnzSc34Lqr8soPAKQL --api-port 8080
    ```

---

## 4. Transactions (`tx`)

Commands related to constructing, signing, and broadcasting operations to the blockchain.

### `send`
Generates a new UTXO transaction, signs the inputs using the sender's private key, and broadcasts the operation to the network's mempool via the P2P swarm.
*   **Required Flags:**
    *   `--from <ADDR>`: Source address (must exist in local `wallet.dat`).
    *   `--to <ADDR>`: Destination address.
    *   `--amount <FLOAT>`: Quantity of SOLE tokens to transfer (decimal precision supported).
*   **Optional Flags:**
    *   `--dry-run`: Constructs and signs the transaction hex but does not broadcast it to the network.
*   **Example:**
    ```bash
    ./sole-cli tx send --from 1HSYNy8yXUuUZrkBCnzSc34Lqr8soPAKQL --to 1SoLErUCu4pL7qrTAouiY4TfWwzAwBsnn --amount 5.5
    ```
