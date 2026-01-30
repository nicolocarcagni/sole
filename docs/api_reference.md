# SOLE API Reference

The SOLE Node exposes a RESTful API for external interaction (Wallets, Explorers, Monitors).

*   **Default Port**: `8080`
*   **Content-Type**: `application/json`

---

## Endpoints

### 1. Check Balance
Retrieves the confirmed balance of a specific address.

**Request**
`GET /balance/{address}`

**Response**
```json
{
  "address": "15U3MUvm16pZSH8WTZHkUw8ngNMjB1pfpw",
  "balance": 500000000 // In Fotoni (Smallest Unit)
}
```

**Example**
```bash
curl -s http://localhost:8080/balance/15U3MUvm16pZSH8WTZHkUw8ngNMjB1pfpw
```

---

### 2. Send Transaction
Broadcasts a signed transaction to the P2P network.

**Request**
`POST /tx/send`

| Field | Type | Description |
| :--- | :--- | :--- |
| `hex` | `string` | The raw, signed transaction bytes encoded in Hex. |

**Response**
```json
{
  "status": "success",
  "txid": "775a47b20d299aceeeaf56b5a7404b2534f84f524aceefd8dc05ee1c29b50d27"
}
```

**Example**
```bash
# Payload must be a JSON object containing the hex string
curl -X POST http://localhost:8080/tx/send \
     -H "Content-Type: application/json" \
     -d '{"hex": "010000..."}'
```

---

### 3. Transaction History
Retrieves all transactions (sent and received) associated with an address.

**Request**
`GET /transactions/{address}`

**Response**
Returns an array of **Rich Transaction Objects**.
```json
[
  {
    "id": "775a47b2...",
    "inputs": [
      {
        "sender_address": "15U3MUvm...",
        "signature": "30450221..."
      }
    ],
    "outputs": [
      {
        "receiver_address": "1J7md...",
        "value": 100000000,
        "value_sole": 1.0
      }
    ],
    "timestamp": 1706572800
  }
]
```

---

### 4. Get Single Transaction
Retrieves details of a specific transaction by its Hash ID (Hex).

**Request**
`GET /transaction/{txid}`

**Response**
Returns a single **Rich Transaction Object**.

```json
{
  "id": "775a47b2...",
  "inputs": [...],
  "outputs": [...],
  "timestamp": 1706572800
}
```

**Errors**
*   `404 Not Found`: If transaction does not exist.

**Example**
```bash
curl -s http://localhost:8080/transaction/775a47b2...
```

**Example**
```bash
curl -s http://localhost:8080/transactions/15U3MUvm16pZSH8WTZHkUw8ngNMjB1pfpw
```

---

---

### 5. Network Peers
Retrieves the list of currently connected P2P peers.

**Request**
`GET /network/peers`

**Response**
```json
{
  "total_peers": 3,
  "peers": [
    "QmXyZ...",
    "QmAbC..."
  ]
}
```

---

### 6. Consensus Validators
Retrieves the list of authorized PoA validators.

**Request**
`GET /consensus/validators`

**Response**
```json
{
  "total_validators": 2,
  "validators": [
    "033cc6...",
    "5b28a2..."
  ]
}
```

---

### 7. Chain Info
Getting the current state of the blockchain tip.

**Request**
`GET /blocks/tip`

**Response**
```json
{
  "height": 105,
  "hash": "3efde2efcf587408ec288bd0cebf4c42da62685fc955dd77272d65fd93a3e7d4"
}
```
