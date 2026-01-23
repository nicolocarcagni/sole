# SOLE Blockchain API Reference

This document provides the official reference for the **SOLE REST API**, a gateway that allows external applications (Web Wallets, Mobile Apps, Explorers) to interact with the blockchain network via standard HTTP/JSON requests.

## 🚀 Activation & Configuration

The API Server runs alongside the P2P node. To enable it, use the `--api-port` flag when starting the node.

*   **Command**: `./sole-cli startnode --api-port 8080`
*   **Default Base URL**: `http://localhost:8080`
*   **CORS**: Enabled by default (`Allow-Origin: *`) to support browser-based applications.

---

## 📡 Endpoints

### 1. Get Chain Status (Tip)
Retrieve the current height and the hash of the latest block (Best Block). Useful for synchronization checks.

*   **Method**: `GET`
*   **Endpoint**: `/blocks/tip`
*   **Description**: Returns the latest block metadata.

#### Example Request
```bash
curl http://localhost:8080/blocks/tip
```

#### Example Response
```json
{
  "height": 42,
  "hash": "0000abc123..."
}
```

---

### 2. Get Address Balance
Retrieve the current balance and UTXO set for a specific address.

*   **Method**: `GET`
*   **Endpoint**: `/balance/{address}`
*   **Parameters**:
    *   `address` (Path): The Base58Check address to query (e.g., `1A1zP1...`).
*   **Description**: Returns the confirmed balance in Fotoni (base unit).

#### Example Request
```bash
curl http://localhost:8080/balance/1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa
```

#### Example Response
```json
{
  "address": "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
  "balance": 150000000
}
```

---

### 3. Get Block Details
Retrieve full details of a specific block by its hash.

*   **Method**: `GET`
*   **Endpoint**: `/blocks/{hash}`
*   **Parameters**:
    *   `hash` (Path): The Hex-encoded hash of the block.
*   **Description**: Returns the block header and list of transactions.

#### Example Request
```bash
curl http://localhost:8080/blocks/7d58f07fe2f7726c15f71b34beaf04fc11038a6cca775bb17aa94323277558bb
```

#### Example Response
```json
{
  "Timestamp": 1768947120,
  "Transactions": [ ... ],
  "PrevBlockHash": "...",
  "Hash": "7d58f0...",
  "Height": 0,
  "Validator": "...",
  "Signature": "..."
}
```

---

### 4. Send Transaction (Broadcast)
Submit a signed transaction to the network.

*   **Method**: `POST`
*   **Endpoint**: `/tx/send`
*   **Description**: Accepts a raw, signed transaction (serialized in Hex), validates it, adds it to the node's Mempool, and broadcasts it to P2P peers.

#### Request Body (JSON)
| Field | Type | Description |
| :--- | :--- | :--- |
| `hex` | String | The full transaction struct serialized to Hex bytes. |

#### Example Request
```bash
curl -X POST http://localhost:8080/tx/send \
     -H "Content-Type: application/json" \
     -d '{"hex": "01000000..."}'
```

#### Example Response
```json
{
  "status": "success",
  "txid": "e3b0c442..."
}
```

#### Error Response
```json
{
  "error": "Transaction invalid"
}
```

---

## 🔄 Example Workflow: Sending from a Light App

This is how a mobile app or web wallet would interact with the SOLE API to send tokens without downloading the blockchain.

1.  **Get Balance & UTXOs**:
    The App calls `GET /balance/{UserAddr}` to know how many tokens are available and which UTXOs to spend.

2.  **Create Transaction (Offline)**:
    The App uses a local library (using the User's Private Key) to:
    *   Select inputs (UTXOs).
    *   Create outputs (Destination + Change).
    *   **Sign** the transaction (ECDSA).
    *   Serialize parameters to a Hex string.

3.  **Broadcast**:
    The App sends the Hex string to the node via `POST /tx/send`.

4.  **Confirm**:
    The node validates the signature and propagates the Tx. The App can poll `/blocks/tip` or listen for the TxID to verify confirmation.
