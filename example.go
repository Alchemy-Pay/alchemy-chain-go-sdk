//go:build ignore
// +build ignore

package main

import (
	"crypto/ecdsa"
	"fmt"

	alchemy "github.com/Alchemy-Pay/alchemy-chain-go-sdk"
	"github.com/ethereum/go-ethereum/crypto"
)

func main() {
	// Configuration
	privateKeyHex := "1234567890123456789012345678901234567890123456789012345678901234"
	alchemy.Config("http://localhost:8545", privateKeyHex)

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

	fmt.Printf("💰 Querying account balance...\n")
	fmt.Printf("📍 Account address: %s\n", userAddress)

	// Query balance
	alchemy.GetBalance(userAddress).
		Success(func(balance *alchemy.BalanceInfo) {
			fmt.Printf("💵 Current ETH balance: %s ETH\n", balance.Eth)
			fmt.Printf("💵 Current Wei balance: %s wei\n\n", balance.Wei)
		}).
		Error(func(err error) {
			fmt.Printf("❌ Balance query failed: %v\n", err)
		})

	// 1. Create token first (this is the only way to get token address)
	alchemy.CreateToken("My Token", "MTK", 8, "0xa6459EF31C68DCF46cC603C526526DB1C6eE4fD1").
		Success(func(response *alchemy.TokenIssueResult) {
			fmt.Printf("✅ Token created successfully!\n")
			fmt.Printf("📋 Token address: %s\n", response.Token)
			fmt.Printf("📋 Transaction hash: %s\n", response.Hash)

			// Get the newly created token address
			tokenAddress := response.Token

			// 2. Get token metadata
			alchemy.GetTokenMetadata(tokenAddress).
				Success(func(metadata *alchemy.TokenMetadata) {
					fmt.Printf("\n📊 Token information:\n")
					fmt.Printf("Name: %s\n", metadata.Name)
					fmt.Printf("Symbol: %s\n", metadata.Symbol)
					fmt.Printf("Decimals: %d\n", metadata.Decimals)
					fmt.Printf("Total supply: %s\n", metadata.Supply)
					fmt.Printf("Is paused: %t\n", metadata.IsPaused)
				}).
				Error(func(err error) {
					fmt.Printf("Failed to get metadata: %v\n", err)
				})

			// 3. Mint tokens
			alchemy.Mint(tokenAddress, "0x1234567890123456789012345678901234567890", "1000000000000000000", 1).
				Success(func(response *alchemy.TransactionResult) {
					fmt.Printf("🪙 Mint successful: %s\n", response.Hash)
				}).
				Error(func(err error) {
					fmt.Printf("Mint failed: %v\n", err)
				})

			// 4. Grant authority
			alchemy.GrantAuthority(tokenAddress, "MINT_ROLE", "0x1234567890123456789012345678901234567890", 2).
				Success(func(response *alchemy.TransactionResult) {
					fmt.Printf("🔐 Authority granted successfully: %s\n", response.Hash)
				}).
				Error(func(err error) {
					fmt.Printf("Authority grant failed: %v\n", err)
				})

			// 5. Update metadata
			alchemy.UpdateMetadata(tokenAddress, "Updated Token Name", "UPD", 3).
				Success(func(response *alchemy.TransactionResult) {
					fmt.Printf("📝 Metadata updated successfully: %s\n", response.Hash)
				}).
				Error(func(err error) {
					fmt.Printf("Metadata update failed: %v\n", err)
				})

			// 6. Revoke authority
			alchemy.RevokeAuthority(tokenAddress, "MINT_ROLE", "0x1234567890123456789012345678901234567890", 4).
				Success(func(response *alchemy.TransactionResult) {
					fmt.Printf("🔒 Authority revoked successfully: %s\n", response.Hash)
				}).
				Error(func(err error) {
					fmt.Printf("Authority revoke failed: %v\n", err)
				})

			// 7. Burn tokens
			alchemy.AdminBurn(tokenAddress, "0x1234567890123456789012345678901234567890", "500000000000000000", 5).
				Success(func(response *alchemy.TransactionResult) {
					fmt.Printf("🔥 Token burned successfully: %s\n", response.Hash)
				}).
				Error(func(err error) {
					fmt.Printf("Token burn failed: %v\n", err)
				})

			// 8. Pause contract
			alchemy.Pause(tokenAddress, 6).
				Success(func(response *alchemy.TransactionResult) {
					fmt.Printf("⏸️ Contract paused successfully: %s\n", response.Hash)
				}).
				Error(func(err error) {
					fmt.Printf("Contract pause failed: %v\n", err)
				})

			// 9. Unpause contract
			alchemy.Unpause(tokenAddress, 7).
				Success(func(response *alchemy.TransactionResult) {
					fmt.Printf("▶️ Contract unpaused successfully: %s\n", response.Hash)
				}).
				Error(func(err error) {
					fmt.Printf("Contract unpause failed: %v\n", err)
				})

			// 10. Add to blacklist
			alchemy.AddToBlacklist(tokenAddress, "0x9999999999999999999999999999999999999999", 8).
				Success(func(response *alchemy.TransactionResult) {
					fmt.Printf("🚫 Added to blacklist successfully: %s\n", response.Hash)
				}).
				Error(func(err error) {
					fmt.Printf("Blacklist addition failed: %v\n", err)
				})

			fmt.Printf("\n🎉 Complete workflow demonstration finished! Token address: %s\n", tokenAddress)
		}).
		Error(func(err error) {
			fmt.Printf("❌ Token creation failed: %v\n", err)
		})
}
