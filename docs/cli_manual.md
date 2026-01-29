# SOLE CLI Manual

The `sole-cli` is the comprehensive tool for managing your node and wallet.

---

## Global Commands

### `init`
Initializes the blockchain database in `./data/blocks`.
*   **Use Condition**: Must be run once before starting a node if the database doesn't exist.
*   **Effect**: Creates Genesis Block (PoA) and **automatically builds the UTXO set**.

### `createwallet`
Generates a new ECDSA keypair.
*   **Output**: Saves keys to `wallet.dat`. Prints new Address (Base58).

### `reindex`
Rebuilds the Persistent UTXO Set from the blockchain history.
*   **Use Condition**: Run this if you upgraded an old node or suspect database corruption.
*   **Effect**: Scans the full chain and repopulates the fast-access UTXO bucket.

---

## Node Operation (`startnode`)

Starts the daemon process, initializing the P2P Host and the HTTP API Gateway.

```bash
./sole-cli startnode [flags]
```

### Flags

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--port` | Int | `3000` | P2P Network listening port (TCP). |
| `--listen` | IP | `0.0.0.0` | Bind Address for P2P listener. |
| `--bootnodes` | String | `""` | Comma-separated list of peer Multiaddrs to bootstrap from. |
| `--public-ip` | IP | `""` | **Public IPv4** to announce. Essential for VPS/NAT traversal. |
| `--public-dns` | String | `""` | **Domain Name** to announce (e.g., `node.sole.com`). |
| `--api-port` | Int | `8080` | REST API Gateway listening port (HTTP). |
| `--miner` | String | `""` | Validator Address. Enables forging. |

### Networking & Discovery

The node uses `libp2p` for decentralized peer discovery.

*   **Identity Persistence**: On first run, the node generates a stable identity key stored in `node_key.dat`. DO NOT lose this file if running a validator/bootnode.
*   **Default Bootnodes**: If `--bootnodes` is omitted, the node automatically connects to the official Unisalento Mainnet (via `sole.nicolocarcagni.dev`).
*   **Local Discovery**: Uses mDNS to find peers on the same LAN.

### Common Scenarios

**Scenario A: Client Domestico (Connecting to Network)**
Simply start the node! It will use default bootnodes.
```bash
./sole-cli startnode
```

**Scenario B: VPS Server / Bootnode (Public Node)**
Configures the node to be reachable from the internet.
*   Binds to all interfaces (`0.0.0.0`).
*   Announces the Public Domain instead of the local IP.
```bash
# Using Domain Name (Recommended for Stability)
./sole-cli startnode \
  --listen 0.0.0.0 \
  --port 3000 \
  --public-dns node.miodominio.com

# Using Raw Public IP
./sole-cli startnode \
  --public-ip 84.22.10.5
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
