# SOLE API Reference
**Building apps on the campus network.**

The SOLE node includes a simple REST API. By default, it listens on port `8080`, but you can change this in your `config.yaml` or with the `--api-port` flag.

## Rate Limiting
*   **Reading data (`GET`)**: 20 requests per second.
*   **Sending actions (`POST`)**: 5 requests per second.

---

### `GET /blocks/tip`
Retrieves the most recent block's parameters (the current chain tip).

*   **Parameters**: None
*   **Response**:
    ```json
    {
      "height": 142,
      "hash": "00af160f81ccd73bf3222f5628f02283d3efc2b24b038c408a2b3df1a4dce26b"
    }
    ```

---

### `GET /blocks/{hash}`
Retrieves total topological parameters and transaction arrays for a specific block hash.

*   **Parameters**:
    *   `hash` (URL Path): 64-character hex-encoded string of the queried block.
*   **Response**:
    ```json
    {
      "timestamp": 1708816000,
      "height": 142,
      "prev_block_hash": "006246d2dcdf635d429ee956b702e45e2e4e3e9317310d5d81e4a76d7774706e",
      "hash": "00af160f81ccd73bf3222f5628f02283d3efc2b24b038c408a2b3df1a4dce26b",
      "transactions": [
        {
          "id": "1a638f8f882ea9bd9b80b9ff9d14e99d2f4249ada1e3cf9fb32bf3039d060131",
          "inputs": [],
          "outputs": [],
          "timestamp": 1708816000
        }
      ],
      "validator": "0499962080b1c07db1ecb7f2d58978203dfe5eede8e648c3755afed392fec7716d8c7a0fe455d15d64b8dd1363d60c78926e9dce4aad2e08a0006cd50215cb87c3",
      "signature": "30440220689..."
    }
    ```

---

### `GET /balance/{address}`
Returns the total Photons available to an address. This is an instant O(1) indexed lookup.

*   **Parameters**:
    *   `address` (URL Path): Base58 check-encoded SOLE address.
*   **Response**:
    ```json
    {
      "address": "1HSYNy8yXUuUZrkBCnzSc34Lqr8soPAKQL",
      "balance": 499900000000000
    }
    ```

---

### `GET /utxos/{address}`
Returns a list of unspent outputs for an address. 

**Mempool Aware:** This endpoint automatically filters out any coins that are currently "pending" in the mempool. This prevents the caller from accidentally trying to double-spend the same coins before a transaction is mined.

*   **Parameters**:
    *   `address` (URL Path): Base58 check-encoded SOLE address.
*   **Response**:
    ```json
    [
      {
        "txid": "1a638f8f882ea9bd9b80b9ff9d14e99d2f4249ada1e3cf9fb32bf3039d060131",
        "vout": 0,
        "amount": 499900000000000
      }
    ]
    ```

---

### `GET /rawtx/{id}`
Returns the raw serialized hex of a transaction. The CLI uses this to verify parent transactions when signing locally.

*   **Parameters**:
    *   `id` (URL Path): 64-character hex-encoded transaction ID.
*   **Response**:
    ```json
    {
      "hex": "01000000018a..."
    }
    ```

---

### `GET /transaction/{id}`
Returns full details for a specific transaction.

*   **Parameters**:
    *   `id` (URL Path): 64-character hex-encoded transaction ID.
*   **Response**:
    ```json
    {
      "id": "1a638f8f882ea9bd9b80b9ff9d14e99d2f4249ada1e3cf9fb32bf3039d060131",
      "inputs": [
        {
          "sender_address": "1HSYNy8yXUuUZrkBCnzSc34Lqr8soPAKQL",
          "signature": "3044..."
        }
      ],
      "outputs": [
        {
          "receiver_address": "1SoLErUCu4pL7qrTAouiY4TfWwzAwBsnn",
          "value": 499900000000000,
          "value_sole": 4999000.0
        },
        {
          "receiver_address": "OP_RETURN: Invoice #812",
          "value": 0,
          "value_sole": 0.0
        }
      ],
      "timestamp": 1708816000
    }
    ```

---

### `GET /transactions/{address}`
Returns all transactions (historical and current) bound to a specific address, either as a sender (input component) or a receiver (output subset).

*   **Parameters**:
    *   `address` (URL Path): Standard Base58 check-encoded SOLE address.
*   **Response**: Returns an array of Transaction Output JSON structures (as specified in `/transaction/{id}`).

---

### `GET /network/peers`
Lists all currently connected nodes mapped through the node's local libp2p swarm host.

*   **Parameters**: None
*   **Response**:
    ```json
    {
      "total_peers": 1,
      "peers": [
        "/ip4/192.168.1.81/tcp/3001/p2p/12D3KooWE6o6RXZaueTmwTjRWf6Dj86k57jnWmAgZW88UMrPhRLG"
      ]
    }
    ```

---

### `GET /consensus/validators`
Yields a statically defined array of authority keys recognized by the node's current binary schema. Validates network security topologies.

*   **Parameters**: None
*   **Response**:
    ```json
    {
      "total_validators": 3,
      "validators": [
        "0499962080b1c07db1ecb..."
      ]
    }
    ```

---

---

### `POST /tx/send`
Submits a raw, properly structured and cryptographically signed hex byte array containing an unconfirmed transaction to the local memory pool.

*   **Headers**: `Content-Type: application/json`
*   **Payload**:
    ```json
    {
      "hex": "01000000018a..."
    }
    ```
*   **Response** (Success):
    ```json
    {
      "status": "success",
      "txid": "7b2e..."
    }
    ```
*   **Response** (Error):
    ```json
    {
      "error": "Transaction invalid"
    }
    ```

---

## Real-time Events (WebSockets)

SOLE v3.0.0 exposes bi-directional communication channels for reactive applications. All events are broadcasted as JSON payloads.

### `/ws/mempool`
Streams all incoming transactions validated by the node. Use this to display "Unconfirmed" activity in real-time.

*   **Event Structure**:
    ```json
    {
      "type": "new_transaction",
      "data": { "id": "...", "amount": 10.5, "sender": "...", "receiver": "..." }
    }
    ```

### `/ws/blocks`
Streams newly forged blocks immediately after they are added to the local chain.

*   **Event Structure**:
    ```json
    {
      "type": "new_block",
      "data": { "height": 143, "hash": "...", "tx_count": 5 }
    }
    ```
