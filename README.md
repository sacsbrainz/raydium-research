# Raydium research Overview

The **Raydium research** outlines the supported pool types, program IDs, and common troubleshooting tips.

## Pool Types

### 1. **Concentrated Liquidity (Suggested)**

- Customizable price ranges for increased capital efficiency.
- Ideal for users seeking better returns on liquidity.

### 2. **Standard AMM**

- Implements the newest CPMM (Constant Product Market Maker).
- Supports Token 2022 standard.
- Offers cheaper transaction costs compared to legacy programs.

### 3. **Legacy AMM v4**

- Based on the legacy AMM program.
- More expensive due to reliance on the orderbook market.
- Provided for compatibility purposes.

## Program IDs

- **Concentrated Liquidity Pool (CLMM):**
  - Program ID: `CAMMCzo5YL8w4VFF8KVHrK22GGUsp5VTaW7grrKgrWqK`

- **Standard AMM Pool (CPMM):**
  - Program ID: `CPMMoo8L3F4NbTegBCKVNunggL7H1ZpdTHKxQB5qKP1C`

- **Legacy AMM v4:**
  - Program ID: `675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8`

## Additional Resources

More solana IDL, see [Bitquery's Solana IDL Library](https://github.com/bitquery/solana-idl-lib).

[Raydium Library GitHub Repository](https://github.com/raydium-io/raydium-library).

## Troubleshooting

### Common Error

```json
{
  "jsonrpc": "2.0",
  "error": {
    "code": -32602,
    "message": "Invalid params: invalid value: map, expected map with a single key."
  },
  "id": 1
}
```

### Cause

This error often arises from an issue with the RPC node, even if the parameters are correct.

### Solution

- Terminate the program.
- Retry after a few seconds or minutes.
- Ensure the RPC node is operational and not experiencing high latency or downtime.
