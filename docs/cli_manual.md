# SOLE CLI Manual

The `sole-cli` is your command center. Use it to manage your wallets, move funds, or run your own node on the SOLE network.

## How to use it

We’ve organized the commands into four main categories: `wallet`, `chain`, `node`, and `tx`. 
Syntax: `./sole-cli <category> <command> [flags]`

## Architecture: Light Client vs. Full Node
Since v3.0.0, the CLI is smart about how it handles data.
*   **Full Node**: Running `./sole-cli node start` turns your machine into a full participant in the network. It downloads the whole chain and handles P2P traffic.
*   **Light Client**: Every other command (like `send` or `balance`) works as a "Light Client." You don't need a local copy of the blockchain. The CLI just talks to a running node via its REST API (default `localhost:8080`). This means you can manage your wallet without locking up your disk space.

---

## 1. Manage Your Wallets (`wallet`)

Create and recover your wallets. We use 12-word mnemonics to keep things simple.

### `create`
Generates a new 12-word mnemonic and sets up your keys. **Write these words down!** If you lose them, you lose your SOLE.
*   **Example:**
    ```bash
    ./sole-cli wallet create
    ```

### `recover`
Did you switch computers? Use this to restore your wallet with your 12 words.
*   **Input:** Type your 12 words separated by spaces.
*   **Example:**
    ```bash
    ./sole-cli wallet recover apple banana cherry ... zebra
    ```

### `list`
See all the addresses you’ve created or imported locally.
*   **Example:**
    ```bash
    ./sole-cli wallet list
    ```

### `balance`
Check how many SOLE you have available to spend. The CLI fetches this data instantly from the Node's API.
*   **Example:**
    ```bash
    ./sole-cli wallet balance --address 1HSYNy8y...
    ```

### `import`
Import a private key to create a new wallet.
*   **Example:**
    ```bash
    ./sole-cli wallet import --key <PRIVATE_KEY>
    ```

### `export`
Export your private key
*   **Example:**
    ```bash
    ./sole-cli wallet export --address <ADDRESS>
    ```

---

## 2. Managing the Chain (`chain`)

These commands help you set up and maintain the local database.

### `init`
Start here. This bootstraps a fresh blockchain database with the Genesis block.
*   **Example:**
    ```bash
    ./sole-cli chain init
    ```

### `print`
Want to see the raw history? This prints every block in the ledger starting from the latest tip.
*   **Example:**
    ```bash
    ./sole-cli chain print
    ```

---

## 3. Running a Node (`node`)

### `start`
This starts the P2P networking and the REST API server. If you’re an authorized validator, providing your address will start the block forging loop.
*   **Key Flags:**
    *   `--miner <ADDR>`: Use this if you are an authorized validator (e.g., Department node).
    *   `--api-port`: Choose which port your apps will use to talk to the node (default 8080).
*   **Example:**
    ```bash
    ./sole-cli node start --miner 1HSYNy8y... --api-port 8080
    ```

---

## 4. Node Configuration (`config.yaml`)

Since v3.0.0, you don't have to pass 10 flags every time you start your node. You can save your settings in a `config.yaml` file in the same folder as the executable.

### Precedence
1. **CLI Flags (Highest):** If you pass a flag (e.g., `--port 3030`), it always overrides everything else. Great for quick tests.
2. **`config.yaml` (Persistent):** Your saved settings for the node.
3. **Hardcoded Defaults (Lowest):** If the flag isn't there and the YAML doesn't mention it, we use the built-in defaults (Port 3000, API 8080).

### Example `config.yaml`
To get started, copy `config.example.yaml` to `config.yaml` and edit it:

```yaml
node:
  port: 3000
  listen: "0.0.0.0"
  miner: "1HSYNy8y..." # Your validator address

network:
  bootnodes: "/ip4/1.2.3.4/tcp/3000/p2p/..."

api:
  port: 8080
```

### System configuration
You just need:

```ini
[Service]
ExecStart=/home/sole/sole-cli node start
WorkingDirectory=/home/sole/
User=sole
```

The node will automatically pick up `config.yaml` from the `WorkingDirectory`.

---

## 5. Sending SOLE (`tx`)

Actually moving value on the network.

### `send`
Create, sign, and broadcast a transaction. 

**Note:** The CLI fetches your unspent outputs from the Node, signs the transaction locally with your private key, and pushes the signed hex back to the Node's mempool. Your private key never leaves your computer.

*   **Required Flags:**
    *   `--from`: Your address (must be in your local wallet).
    *   `--to`: Who are you sending to?
    *   `--amount`: How many SOLE?
*   **Optional Flags:**
    *   `--memo`: Add a message (max 80 bytes).
*   **Example:**
    ```bash
    ./sole-cli tx send --from 1HSYNy... --to 1SoLEr... --amount 15.0 --memo "Notes for Calculus I"
    ```
