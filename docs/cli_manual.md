# SOLE CLI Reference Manual

The `sole-cli` is the command-line interface for interacting with the SOLE network.

## Global Flags
*   `--help`: Show help for any command.

---

## 🏗 Initialization

### `init`
Initializes a new local blockchain node with the official Unisalento Genesis Block.
*   **Usage**: `./sole-cli init`
*   **Description**: Creates the database files (`tmp/blocks`) and writes the hardcoded Genesis Block. Safe to run; if the DB exists, it will exit gracefully.
*   **Example Output**:
    ```
    ☀️  SOLE Blockchain Inizializzata!
    - Genesis Hash: 7d58f0...
    - Network: Unisalento Mainnet
    ```

---

## 💰 Wallet Management

### `createwallet`
Generates a new ECDSA key pair and saves it to `wallet.dat`.
*   **Usage**: `./sole-cli createwallet`
*   **Description**: Creates a new address for receiving funds.
*   **Example Output**: `Nuovo portafoglio creato: 1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa`

### `listaddresses`
Lists all addresses stored in the local `wallet.dat` file.
*   **Usage**: `./sole-cli listaddresses`
*   **Description**: Useful for seeing all your available accounts.

### `getbalance`
Checks the balance of a specific address.
*   **Usage**: `./sole-cli getbalance --address <ADDRESS>`
*   **Description**: Scans the local blockchain copy for UTXOs belonging to the address.
*   **Requires**: Local blockchain database (run `init` or `startnode` first).

### `printwallet`
Exports the sensitive keys for a specific address.
*   **Usage**: `./sole-cli printwallet --address <ADDRESS>`
*   **Warning**: Displays the **Private Key** on screen. Use with caution.
*   **Use Case**: Backups or configuring a Validator node.

---

## 💸 Transactions

### `send`
Transfers tokens from one wallet to another.
*   **Usage**: `./sole-cli send --from <SENDER> --to <RECEIVER> --amount <AMOUNT>`
*   **Flags**:
    *   `--from`: Sender Address (must be in local `wallet.dat`).
    *   `--to`: Receiver Address.
    *   `--amount`: Integer amount of SOLE to send.
*   **Behavior**: Creates a transaction, attempts to discover peers via P2P user `mDNS`, broadcast the TX to the network, and waits for confirmation.

---

## 🌐 Node Operation

### `startnode`
Starts the P2P node daemon.
*   **Usage**: `./sole-cli startnode [flags]`
*   **Flags**:
    *   `--port`: TCP port to listen on (Default: 3000).
    *   `--miner <ADDRESS>`: Enable Validator/Forging mode (must use an authorized address).
*   **Description**: Connects to the network, synchronizes blocks, and (if configured) forges new blocks.

### `printchain`
Dumps the entire local blockchain to stdout.
*   **Usage**: `./sole-cli printchain`
*   **Description**: Useful for debugging and verifying chain state (Heights, Hashes, Confirmations).

---

## 🔧 Troubleshooting

### "Error: Wallet not found"
Ensure `wallet.dat` exists in the current directory. Run `createwallet` if needed.

### "Blockchain already exists"
If `init` errors, it means you already have a database. This is fine. If you want to wipe it and start fresh, run `rm -rf tmp/` (Warning: Irreversible).

### Node not connecting
*   Ensure both nodes are on the same network (if using mDNS).
*   Check firewall settings for the port (Default 3000).
*   Ensure both nodes are running the same binary version.
