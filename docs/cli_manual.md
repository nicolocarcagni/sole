# SOLE CLI Manual

The **SOLE CLI** (`sole-cli`) is the unified command-line interface for interacting with the SOLE Blockchain. It allows you to manage wallets, operate a full node, inspect the blockchain, and send transactions.

---

## Introduction

The CLI uses a **resource-based** structure, similar to `kubectl` or `docker`.
The general syntax is:

```bash
./sole-cli <RESOURCE> <ACTION> [FLAGS]
```

**Available Resources:**
*   `wallet`: Manage keys and addresses.
*   `chain`: Initialize and inspect the blockchain database.
*   `node`: Run the P2P daemon and miner.
*   `tx`: Create and broadcast transactions.

---

## Wallet Management (`wallet`)

Manage your ECDSA key pairs and addresses.

### Create a New Wallet
Generates a new private key and saves it to `wallet.dat`.

```bash
./sole-cli wallet create
```

### List Addresses
Displays all addresses stored in the local wallet file.

```bash
./sole-cli wallet list
```

### Import a Private Key
Imports an existing ECDSA P-256 private key (in Hex format).

```bash
./sole-cli wallet import --key <PRIVATE_KEY_HEX>
```

### Remove a Wallet
Removes a specific address and its key from the local storage.
**Warning**: This action is irreversible if you don't have a backup.

```bash
./sole-cli wallet remove --address <ADDRESS>
```

### Export Private Key
Reveals the private key for a specific address.

```bash
./sole-cli wallet export --address <ADDRESS>
```

### Check Balance
Checks the balance of an address by scanning the local UTXO set.

```bash
./sole-cli wallet balance --address <ADDRESS>
```

---

## Blockchain Management (`chain`)

Manage the local blockchain database (`./data/blocks`).

### Initialize Blockchain
Initializes the database with the Genesis Block.
**Run this once before starting the node.**

```bash
./sole-cli chain init
```

### Reindex UTXO Set
Rebuilds the Unspent Transaction Output (UTXO) set from the full block history. Use this if the database state seems inconsistent.

```bash
./sole-cli chain reindex
```

### Print Blockchain
Prints the full history of blocks to stdout.

```bash
./sole-cli chain print
```

### Reset Blockchain
**DANGER**: Deletes the entire blockchain database. Requires confirmation.

```bash
./sole-cli chain reset
```

---

## Node Operations (`node`)

Operate the P2P node.

### Start Node
Starts the P2P daemon. This command blocks until the node is stopped (Ctrl+C).

```bash
./sole-cli node start [flags]
```

#### Networking Flags

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--port` | Int | `3000` | P2P Listen Port (TCP). |
| `--listen` | IP | `0.0.0.0` | Bind Address. |
| `--bootnodes` | String | `""` | Comma-separated list of bootstrap peers. |
| `--public-ip` | IP | `""` | Public IPv4 to announce (for NAT/VPS). |
| `--public-dns` | String | `""` | Domain name to announce (e.g., `node.sole.com`). |

#### Mining Flags

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--miner` | Address | `""` | Address to credit mining rewards to. Enables mining. |

#### API Flags

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--api-port` | Int | `8080` | HTTP API Port. |
| `--api-listen` | IP | `0.0.0.0` | HTTP API Bind Address. |

---

## Transactions (`tx`)

Create and broadcast transactions.

### Send Funds
Sends SOLE coins from one address to another.

```bash
./sole-cli tx send --from <SENDER> --to <RECEIVER> --amount <AMOUNT>
```

*   `--amount`: Value in SOLE (decimal allowed).
*   `--dry-run`: If set, prints the signed transaction hex without broadcasting.

---

## Shell Autocompletion (`completion`)

Generate autocompletion scripts for your shell.

**Bash:**
```bash
source <(./sole-cli completion bash)
```

**Zsh:**
```bash
source <(./sole-cli completion zsh)
```
