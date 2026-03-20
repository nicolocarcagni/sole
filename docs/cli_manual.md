# SOLE CLI Manual

The `sole-cli` is your command center. Use it to manage your wallets, move funds, or run your own node on the SOLE network.

## How to use it

We’ve organized the commands into four main categories: `wallet`, `chain`, `node`, and `tx`. 
Syntax: `./sole-cli <category> <command> [flags]`

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
Check how many SOLE you have available to spend.
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

## 4. Sending SOLE (`tx`)

Actually moving value on the network.

### `send`
Create, sign, and broadcast a transaction.
*   **Required Flags:**
    *   `--from`: Your address (must be in your local wallet).
    *   `--to`: Who are you sending to?
    *   `--amount`: How many SOLE?
*   **Optional Flags:**
    *   `--memo`: Add a message (max 80 bytes). Great for "Coffee at faculty bar" or "Math notes".
*   **Example:**
    ```bash
    ./sole-cli tx send --from 1HSYNy... --to 1SoLEr... --amount 15.0 --memo "Notes for Calculus I"
    ```
