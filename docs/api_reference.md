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

### 3. Chain Info
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
