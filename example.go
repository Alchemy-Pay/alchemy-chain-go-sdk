//go:build ignore
// +build ignore

// 用法：go run example.go
//
// userNonce 由 SDK 自动管理：首次操作时从服务端拉取初始值并缓存，
// 后续调用本地自增，无论成败都消耗当次 nonce。
// 若服务端返回 nonce 不匹配（如服务重启），SDK 自动重同步后重试一次。
// 调用方无需手动管理 nonce，也无需在每次操作前查询 GetUserNonce。

package main

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"

	alchemy "github.com/Alchemy-Pay/alchemy-chain-go-sdk"
	"github.com/ethereum/go-ethereum/crypto"
)

// getACHBalance 查询用户在 ACH token 合约中的余额（8位精度最小单位）。
func getACHBalance(achTokenAddress, userAddress string) *big.Int {
	var balance *big.Int
	alchemy.GetTokenBalance(achTokenAddress, userAddress).
		Success(func(b *alchemy.TokenBalanceInfo) {
			balance, _ = new(big.Int).SetString(b.Amount, 10)
		})
	return balance
}

// getNativeACHBalance 通过 GetBalance（eth_getBalance）查询账户原生 ACH 余额，与 BalanceInfo.Wei 一致。
func getNativeACHBalance(userAddress string) *big.Int {
	var balance *big.Int
	alchemy.GetBalance(userAddress).
		Success(func(b *alchemy.BalanceInfo) {
			balance, _ = new(big.Int).SetString(b.Wei, 10)
		})
	return balance
}

// printGasUsed 打印本次操作消耗的原生 ACH gas（before - after，最小单位 wei）。
func printGasUsed(op string, before, after *big.Int) {
	if before == nil || after == nil {
		fmt.Printf("[Gas] %-20s (balance unavailable)\n", op+":")
		return
	}
	used := new(big.Int).Sub(before, after)
	fmt.Printf("[Gas] %-20s %s (native ACH wei)\n", op+":", used.String())
}

func main() {
	privateKeyHex := "***"

	// 初始化 SDK（第三个参数可选，覆盖默认 chain ID 202512）
	alchemy.Config("https://rpc.testnet.alchemychain.com", privateKeyHex)

	// 从私钥派生调用者地址
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		fmt.Printf("private key error: %v\n", err)
		return
	}
	publicKeyECDSA, ok := privateKey.Public().(*ecdsa.PublicKey)
	if !ok {
		fmt.Println("public key conversion failed")
		return
	}
	userAddress := crypto.PubkeyToAddress(*publicKeyECDSA).Hex()
	fmt.Printf("Account  : %s\n", userAddress)

	// 查询当前 userNonce（仅用于调试/监控，操作时 SDK 自动管理，无需手动传入）
	alchemy.GetUserNonce(userAddress).
		Success(func(n int64) {
			fmt.Printf("UserNonce: %d (auto-managed by SDK)\n", n)
		}).
		Error(func(err error) {
			fmt.Printf("GetUserNonce failed: %v\n", err)
		})

	initialBalance := getNativeACHBalance(userAddress)
	if initialBalance != nil {
		fmt.Printf("Native ACH balance (wei): %s\n\n", initialBalance.String())
	} else {
		fmt.Println("GetBalance failed, native ACH balance unknown")
	}

	// ── 1. CreateToken ───────────────────────────────────────────────────────
	var tokenAddress string
	alchemy.CreateToken("My Token", "MTK", 8, userAddress).
		Success(func(r *alchemy.TokenIssueResult) {
			tokenAddress = r.Token
			fmt.Printf("[CreateToken] token=%s\n             tx=%s\n\n", r.Token, r.Hash)
		}).
		Error(func(err error) {
			fmt.Printf("[CreateToken] failed: %v\n", err)
		})

	if tokenAddress == "" {
		fmt.Println("CreateToken failed, stopping.")
		return
	}

	// ── 2. Mint ──────────────────────────────────────────────────────────────
	before := getNativeACHBalance(userAddress)
	alchemy.Mint(tokenAddress, userAddress, "1000000000000000000"). // 1 token（18位精度）
		Success(func(r *alchemy.TransactionResult) {
			printGasUsed("Mint", before, getNativeACHBalance(userAddress))
			fmt.Printf("[Mint]        tx=%s\n\n", r.Hash)
		}).
		Error(func(err error) { fmt.Printf("[Mint] failed: %v\n", err) })

	// ── 3. GetTokenMetadata ──────────────────────────────────────────────────
	alchemy.GetTokenMetadata(tokenAddress).
		Success(func(m *alchemy.TokenMetadata) {
			fmt.Printf("[Metadata]    name=%s symbol=%s decimals=%d supply=%s paused=%v\n\n",
				m.Name, m.Symbol, m.Decimals, m.Supply, m.IsPaused)
		}).
		Error(func(err error) { fmt.Printf("[GetTokenMetadata] failed: %v\n", err) })

	// ── 4. UpdateMetadata ────────────────────────────────────────────────────
	before = getNativeACHBalance(userAddress)
	alchemy.UpdateMetadata(tokenAddress, "Updated Token", "UPD").
		Success(func(r *alchemy.TransactionResult) {
			printGasUsed("UpdateMetadata", before, getNativeACHBalance(userAddress))
			fmt.Printf("[UpdateMeta]  tx=%s\n\n", r.Hash)
		}).
		Error(func(err error) { fmt.Printf("[UpdateMetadata] failed: %v\n", err) })

	// ── 5. GrantAuthority ────────────────────────────────────────────────────
	before = getNativeACHBalance(userAddress)
	alchemy.GrantAuthority(tokenAddress, "MINT_ROLE", userAddress).
		Success(func(r *alchemy.TransactionResult) {
			printGasUsed("GrantAuthority", before, getNativeACHBalance(userAddress))
			fmt.Printf("[Grant]       tx=%s\n\n", r.Hash)
		}).
		Error(func(err error) { fmt.Printf("[GrantAuthority] failed: %v\n", err) })

	// ── 6. RevokeAuthority ───────────────────────────────────────────────────
	before = getNativeACHBalance(userAddress)
	alchemy.RevokeAuthority(tokenAddress, "MINT_ROLE", userAddress).
		Success(func(r *alchemy.TransactionResult) {
			printGasUsed("RevokeAuthority", before, getNativeACHBalance(userAddress))
			fmt.Printf("[Revoke]      tx=%s\n\n", r.Hash)
		}).
		Error(func(err error) { fmt.Printf("[RevokeAuthority] failed: %v\n", err) })

	// ── 7. AdminBurn ─────────────────────────────────────────────────────────
	before = getNativeACHBalance(userAddress)
	alchemy.AdminBurn(tokenAddress, userAddress, "500000000000000000"). // 0.5 token
		Success(func(r *alchemy.TransactionResult) {
			printGasUsed("AdminBurn", before, getNativeACHBalance(userAddress))
			fmt.Printf("[AdminBurn]   tx=%s\n\n", r.Hash)
		}).
		Error(func(err error) { fmt.Printf("[AdminBurn] failed: %v\n", err) })

	// ── 8. AddToBlacklist ────────────────────────────────────────────────────
	blacklistTarget := "0x9999999999999999999999999999999999999999"
	before = getNativeACHBalance(userAddress)
	alchemy.AddToBlacklist(tokenAddress, blacklistTarget).
		Success(func(r *alchemy.TransactionResult) {
			printGasUsed("AddToBlacklist", before, getNativeACHBalance(userAddress))
			fmt.Printf("[Blacklist]   tx=%s\n\n", r.Hash)
		}).
		Error(func(err error) { fmt.Printf("[AddToBlacklist] failed: %v\n", err) })

	// ── 9. Pause ─────────────────────────────────────────────────────────────
	before = getNativeACHBalance(userAddress)
	alchemy.Pause(tokenAddress).
		Success(func(r *alchemy.TransactionResult) {
			printGasUsed("Pause", before, getNativeACHBalance(userAddress))
			fmt.Printf("[Pause]       tx=%s\n\n", r.Hash)
		}).
		Error(func(err error) { fmt.Printf("[Pause] failed: %v\n", err) })

	// ── 10. Unpause ──────────────────────────────────────────────────────────
	before = getNativeACHBalance(userAddress)
	alchemy.Unpause(tokenAddress).
		Success(func(r *alchemy.TransactionResult) {
			printGasUsed("Unpause", before, getNativeACHBalance(userAddress))
			fmt.Printf("[Unpause]     tx=%s\n\n", r.Hash)
		}).
		Error(func(err error) { fmt.Printf("[Unpause] failed: %v\n", err) })

	// ── 汇总 ─────────────────────────────────────────────────────────────────
	finalBalance := getNativeACHBalance(userAddress)
	if finalBalance != nil {
		fmt.Printf("Native ACH balance (wei): %s\n\n", finalBalance.String())
	}
	if initialBalance != nil && finalBalance != nil {
		totalGas := new(big.Int).Sub(initialBalance, finalBalance)
		fmt.Printf("Total native ACH gas consumed (wei): %s\n", totalGas.String())
	}
}
