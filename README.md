# @alchemy-chain/go-sdk

Alchemy Chain Go SDK - A simplified token API toolkit for blockchain token operations and interactions.

## Installation

```bash
go get github.com/alchemy-chain/go-sdk
```

## Quick Start

```go
package main

import (
    "fmt"
    alchemy "github.com/alchemy-chain/go-sdk"
)

func main() {
    // Configure RPC endpoint and private key
    alchemy.Config("https://your-rpc-endpoint.com", "your-private-key")
    
    // Create token
    alchemy.CreateToken("My Token", "MTK", 18, "0x...masterAuthority").
        Success(func(result *alchemy.TokenIssueResult) {
            fmt.Printf("Token address: %s\n", result.Token)
            fmt.Printf("Transaction hash: %s\n", result.Hash)
        }).
        Error(func(err error) {
            fmt.Printf("Creation failed: %v\n", err)
        })
}
```

## API Documentation

### Configuration

#### `Config(rpcUrl, privateKey string)`

Configure RPC endpoint and private key.

- `rpcUrl`: RPC endpoint URL
- `privateKey`: Private key for signing (can include or exclude 0x prefix)

### Token Operations

#### `CreateToken(name, symbol string, decimals int32, masterAuthority string) *ResponseHandler[*TokenIssueResult]`

Create a new token.

- `name`: Token name
- `symbol`: Token symbol  
- `decimals`: Number of decimal places
- `masterAuthority`: Master authority address

**Returns**: ResponseHandler with `.Success()` and `.Error()` methods.

#### `GetTokenMetadata(tokenAddress string) *ResponseHandler[*TokenMetadata]`

Get token metadata.

- `tokenAddress`: Token contract address

#### `UpdateMetadata(tokenAddress, newName, newSymbol string, nonce int64) *ResponseHandler[*TransactionResult]`

Update token metadata.

- `tokenAddress`: Token contract address
- `newName`: New token name
- `newSymbol`: New token symbol
- `nonce`: Transaction nonce value

#### `Mint(tokenAddress, toAddress, amount string, nonce int64) *ResponseHandler[*TransactionResult]`

Mint tokens.

- `tokenAddress`: Token contract address
- `toAddress`: Recipient address
- `amount`: Amount to mint (wei value as string)
- `nonce`: Transaction nonce value

#### `AdminBurn(tokenAddress, fromAddress, amount string, nonce int64) *ResponseHandler[*TransactionResult]`

Admin burn tokens.

- `tokenAddress`: Token contract address
- `fromAddress`: Address to burn tokens from
- `amount`: Amount to burn (wei value as string)
- `nonce`: Transaction nonce value

### Authority Management

#### `GrantAuthority(tokenAddress, role, account string, nonce int64) *ResponseHandler[*TransactionResult]`

Grant authority.

- `tokenAddress`: Token contract address
- `role`: Authority role (e.g. "MINT_ROLE")
- `account`: Account address to be granted authority
- `nonce`: Transaction nonce value

#### `RevokeAuthority(tokenAddress, role, account string, nonce int64) *ResponseHandler[*TransactionResult]`

Revoke authority.

- `tokenAddress`: Token contract address
- `role`: Authority role
- `account`: Account address to revoke authority from
- `nonce`: Transaction nonce value

### Contract Control

#### `Pause(tokenAddress string, nonce int64) *ResponseHandler[*TransactionResult]`

Pause contract.

- `tokenAddress`: Token contract address
- `nonce`: Transaction nonce value

#### `Unpause(tokenAddress string, nonce int64) *ResponseHandler[*TransactionResult]`

Unpause contract.

- `tokenAddress`: Token contract address
- `nonce`: Transaction nonce value

#### `AddToBlacklist(tokenAddress, accountAddress string, nonce int64) *ResponseHandler[*TransactionResult]`

Add account to blacklist.

- `tokenAddress`: Token contract address
- `accountAddress`: Account address to add to blacklist
- `nonce`: Transaction nonce value

### Utility Methods

#### `GetBalance(address string) *ResponseHandler[*BalanceInfo]`

Get ETH balance.

- `address`: Address to query

**Returns**: ResponseHandler that returns BalanceInfo with `Wei` and `Eth` fields on success.

## Data Structures

### TokenMetadata
```go
type TokenMetadata struct {
    Name     string `json:"name"`
    Symbol   string `json:"symbol"`
    Decimals uint8  `json:"decimals"`
    Supply   string `json:"supply"`
    IsPaused bool   `json:"isPaused"`
}
```

### TokenIssueResult
```go
type TokenIssueResult struct {
    Hash  string `json:"hash"`
    Token string `json:"token"`
}
```

### TransactionResult
```go
type TransactionResult struct {
    Hash string `json:"hash"`
}
```

### BalanceInfo
```go
type BalanceInfo struct {
    Wei string `json:"wei"`
    Eth string `json:"eth"`
}
```

## Examples

See `example.go` file for complete usage examples.

```go
package main

import (
    "crypto/ecdsa"
    "fmt"
    alchemy "github.com/alchemy-chain/go-sdk"
    "github.com/ethereum/go-ethereum/crypto"
)

func main() {
    // Configuration
    privateKeyHex := "your-private-key"
    alchemy.Config("https://validator-node-rpc.aeon.xyz", privateKeyHex)
    
    // Get address from private key
    privateKey, err := crypto.HexToECDSA(privateKeyHex)
    if err != nil {
        fmt.Printf("❌ Private key conversion failed: %v\n", err)
        return
    }
    
    publicKey := privateKey.Public()
    publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
    if !ok {
        fmt.Printf("❌ Public key conversion failed\n")
        return
    }
    
    userAddress := crypto.PubkeyToAddress(*publicKeyECDSA).Hex()
    
    // Query balance
    alchemy.GetBalance(userAddress).
        Success(func(balance *alchemy.BalanceInfo) {
            fmt.Printf("💵 Current ETH balance: %s ETH\n", balance.Eth)
            fmt.Printf("💵 Current Wei balance: %s wei\n\n", balance.Wei)
        }).
        Error(func(err error) {
            fmt.Printf("❌ Balance query failed: %v\n", err)
        })
    
    // Create token
    alchemy.CreateToken("My Token", "MTK", 8, "0xa6459EF31C68DCF46cC603C526526DB1C6eE4fD1").
        Success(func(response *alchemy.TokenIssueResult) {
            fmt.Printf("✅ Token creation successful!\n")
            fmt.Printf("📋 Token address: %s\n", response.Token)
            fmt.Printf("📋 Transaction hash: %s\n", response.Hash)
            
            tokenAddress := response.Token
            
            // Get token metadata
            alchemy.GetTokenMetadata(tokenAddress).
                Success(func(metadata *alchemy.TokenMetadata) {
                    fmt.Printf("\n📊 Token Information:\n")
                    fmt.Printf("Name: %s\n", metadata.Name)
                    fmt.Printf("Symbol: %s\n", metadata.Symbol)
                    fmt.Printf("Decimals: %d\n", metadata.Decimals)
                }).
                Error(func(err error) {
                    fmt.Printf("Get metadata failed: %v\n", err)
                })
                
            // Mint tokens
            alchemy.Mint(tokenAddress, "0x...", "1000000000000000000", 1).
                Success(func(response *alchemy.TransactionResult) {
                    fmt.Printf("🪙 Mint successful: %s\n", response.Hash)
                }).
                Error(func(err error) {
                    fmt.Printf("Mint failed: %v\n", err)
                })
        }).
        Error(func(err error) {
            fmt.Printf("❌ Token creation failed: %v\n", err)
        })
}
```

## Important Notes

1. **Private Key Security**: Please keep your private key secure and do not hardcode it in your code
2. **Nonce Management**: Ensure you use the correct nonce value when calling methods that require nonce
3. **Network Configuration**: Make sure the RPC endpoint is accessible and compatible
4. **Error Handling**: It's recommended to add appropriate error handling for all operations
5. **Dependencies**: This SDK requires `github.com/ethereum/go-ethereum` for cryptographic functions

## Dependencies

```bash
go mod tidy
```

The SDK automatically manages the following dependencies:
- `github.com/ethereum/go-ethereum` - Ethereum cryptographic functions

## License

MIT

## Contributing

Issues and pull requests are welcome!
