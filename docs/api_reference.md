# SOLE API Reference

The SOLE node exposes a RESTful JSON HTTP server for managing transactions, checking chain topologies, and retrieving network heuristics. By default, it listens on port `8080`.

## Rate Limiting
To ensure reliable operation across distributed network topographies, endpoints are rate-limited per connecting IP:
*   **Data Endpoints (`GET`)**: 20 requests/sec (burst tolerance: 30)
*   **Action Endpoints (`POST`)**: 5 requests/sec (burst tolerance: 10)

All successful queries yield JSON payloads. Endpoints return precise JSON mappings for cryptographic components.

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
Queries the UTXO set for the aggregate confirmed balance available to an address.

*   **Parameters**:
    *   `address` (URL Path): Standard Base58 check-encoded SOLE address.
*   **Response**:
    ```json
    {
      "address": "1HSYNy8yXUuUZrkBCnzSc34Lqr8soPAKQL",
      "balance": 499900000000000
    }
    ```

---

### `GET /utxos/{address}`
Retrieves a flat list of unspent transaction outputs explicitly locked to the referenced address. Essential for external wallet SDKs attempting to construct off-grid transaction structures manually.

*   **Parameters**:
    *   `address` (URL Path): Standard Base58 check-encoded SOLE address.
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

### `GET /transaction/{id}`
Extracts full input, output array bindings, and value routes (both Photons and fractional SOLE implementations) for a specific transaction ID.

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
