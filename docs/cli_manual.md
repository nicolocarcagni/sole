# SOLE CLI Manual

The `sole-cli` is the comprehensive tool for managing your node and wallet.

---

## Global Commands

### `init`
Initializes the blockchain database in `./data/blocks`.
*   **Use Condition**: Must be run once before starting a node if the database doesn't exist.
*   **Effect**: Creates Genesis Block (PoA).

### `createwallet`
Generates a new ECDSA keypair.
*   **Output**: Saves keys to `wallet.dat`. Prints new Address (Base58).

---

## Node Operation (`startnode`)

Starts the daemon process.

```bash
./sole-cli startnode [flags]
```

### Flags

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--port` | `3000` | P2P Network listening port (TCP). |
| `--api-port` | `8080` | REST API Gateway listening port (HTTP). |
| `--miner` | `""` | Validator Address. If set, node attempts to forge new blocks. |

**Example: Validator Mode**
```bash
./sole-cli startnode --port 3000 --api-port 8080 --miner 15U3MUvm...
```

---

## Transaction Tools

### `send`
Utilities for transferring funds.

```bash
./sole-cli send --from <ADDR> --to <ADDR> --amount <INT> [flags]
```

**Options**
*   **`--dry-run`**: Prints the raw signed transaction Hex to stdout **without** broadcasting it. Useful for API integration or offline signing.
