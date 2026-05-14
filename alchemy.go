package alchemy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// Global configuration
var (
	baseURL    = "http://localhost:8545"
	privateKey = ""
	chainID    = int64(2025) // ACH Chain ID
	client     = &http.Client{Timeout: 30 * time.Second}
)

// 本地 nonce 缓存：首次从服务端拉取，之后本地自增，无论业务成败都消耗（方案 B）。
var (
	nonceCacheMu sync.Mutex
	nonceCache   = make(map[string]*addrNonce) // key: lowercase address
)

type addrNonce struct {
	initialized bool
	value       int64
}

// allocateNonce 原子地分配下一个 nonce 并自增本地计数。
// 首次调用时从服务端拉取初始值；后续直接本地自增，不再查服务端。
func allocateNonce(address string) (int64, error) {
	key := strings.ToLower(address)

	nonceCacheMu.Lock()
	an, ok := nonceCache[key]
	if !ok {
		an = &addrNonce{}
		nonceCache[key] = an
	}

	if !an.initialized {
		// 需要从服务端拉取，先释放锁避免阻塞其他地址
		nonceCacheMu.Unlock()
		serverNonce, err := fetchNonceFromServer(address)
		nonceCacheMu.Lock()
		if err != nil {
			nonceCacheMu.Unlock()
			return 0, err
		}
		if !an.initialized { // double-check，防止并发初始化
			an.value = serverNonce
			an.initialized = true
		}
	}

	nonce := an.value
	an.value++
	nonceCacheMu.Unlock()
	return nonce, nil
}

// resyncNonce 从服务端重新获取 nonce 并覆盖本地缓存，用于错误恢复。
func resyncNonce(address string) error {
	serverNonce, err := fetchNonceFromServer(address)
	if err != nil {
		return err
	}
	key := strings.ToLower(address)
	nonceCacheMu.Lock()
	if an, ok := nonceCache[key]; ok {
		an.value = serverNonce
		an.initialized = true
	} else {
		nonceCache[key] = &addrNonce{initialized: true, value: serverNonce}
	}
	nonceCacheMu.Unlock()
	return nil
}

// isNonceMismatch 判断错误是否为服务端 nonce 不匹配（需重同步后重试）。
func isNonceMismatch(err error) bool {
	return err != nil && strings.Contains(err.Error(), "invalid userNonce")
}

// Config configures the API endpoint and private key.
// An optional third argument overrides the default chain ID (202512).
func Config(url, key string, cid ...int64) {
	baseURL = url
	privateKey = key
	if len(cid) > 0 {
		chainID = cid[0]
	}
}

// ResponseHandler handles responses with success/error callbacks
type ResponseHandler[T any] struct {
	data T
	err  error
}

func (r *ResponseHandler[T]) Success(callback func(T)) *ResponseHandler[T] {
	if r.err == nil {
		callback(r.data)
	}
	return r
}

func (r *ResponseHandler[T]) Error(callback func(error)) *ResponseHandler[T] {
	if r.err != nil {
		callback(r.err)
	}
	return r
}

// Data structures
type TokenMetadata struct {
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Decimals uint8  `json:"decimals"`
	Supply   string `json:"supply"`
	IsPaused bool   `json:"isPaused"`
}

type TokenIssueResult struct {
	Hash  string `json:"hash"`
	Token string `json:"token"`
}

type TransactionResult struct {
	Hash string `json:"hash"`
}

// Signature represents cryptographic signature
type Signature struct {
	R string `json:"r"`
	S string `json:"s"`
	V string `json:"v"`
}

// buildSortedMessage creates a signing message with format key=value joined by &.
// Arrays are expanded as key[0]=v0&key[1]=v1 to eliminate field-boundary ambiguity.
// This matches the server-side implementation in crypto/signature.go (Finding 12).
func buildSortedMessage(params map[string]interface{}) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var messageParts []string
	for _, key := range keys {
		value := params[key]
		if value == nil {
			continue
		}

		if slice, ok := value.([]interface{}); ok {
			for i, element := range slice {
				if element != nil {
					messageParts = append(messageParts, fmt.Sprintf("%s[%d]=%v", key, i, element))
				}
			}
		} else {
			messageParts = append(messageParts, fmt.Sprintf("%s=%v", key, value))
		}
	}

	return strings.Join(messageParts, "&")
}

// generateSignature universal signing method - sorts keys a-z then signs
func generateSignature(params map[string]interface{}) (*Signature, error) {
	privateKeyHex := privateKey
	if len(privateKeyHex) > 2 && privateKeyHex[:2] == "0x" {
		privateKeyHex = privateKeyHex[2:]
	}

	privKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	message := buildSortedMessage(params)

	hash := crypto.Keccak256Hash([]byte(message))

	signature, err := crypto.Sign(hash.Bytes(), privKey)
	if err != nil {
		return nil, fmt.Errorf("sign error: %w", err)
	}

	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:64])
	v := new(big.Int).SetBytes([]byte{signature[64] + 27})

	return &Signature{
		R: r.String(),
		S: s.String(),
		V: v.String(),
	}, nil
}

// getCallerAddress derives the caller address from the configured private key.
func getCallerAddress() (string, error) {
	privateKeyHex := privateKey
	if len(privateKeyHex) > 2 && privateKeyHex[:2] == "0x" {
		privateKeyHex = privateKeyHex[2:]
	}
	privKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return "", fmt.Errorf("invalid private key: %w", err)
	}
	return crypto.PubkeyToAddress(privKey.PublicKey).Hex(), nil
}

// fetchNonceFromServer 直接从服务端 GET /usernonce 查询当前期望 nonce。
// 服务端已修复为返回 max(链上nonce, 内存屏障)，结果是真实的下一个可用 nonce。
// 不可达时回退到 0（兼容未部署 UserNonceRegistry 的场景）。
func fetchNonceFromServer(address string) (int64, error) {
	resp, err := client.Get(baseURL + "/usernonce?address=" + address)
	if err != nil {
		return 0, nil // fallback: registry not reachable
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, nil // fallback: registry not configured
	}

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		UserNonce uint64 `json:"userNonce"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, nil
	}
	return int64(result.UserNonce), nil
}

// GetUserNonce 查询指定地址当前的期望 userNonce（直接查服务端，不走本地缓存）。
func GetUserNonce(address string) *ResponseHandler[int64] {
	nonce, err := fetchNonceFromServer(address)
	if err != nil {
		return &ResponseHandler[int64]{err: err}
	}
	return &ResponseHandler[int64]{data: nonce}
}

// CreateToken creates a new token.
// The signing message includes chainId, userNonce, and all token parameters.
// userNonce 由本地缓存自动分配（首次从服务端拉取，之后本地自增）。
// 遇到 nonce 不匹配错误时，重同步后自动重试一次。
func CreateToken(name, symbol string, decimals int32, masterAuthority string) *ResponseHandler[*TokenIssueResult] {
	result := attemptCreateToken(name, symbol, decimals, masterAuthority)
	if isNonceMismatch(result.err) {
		if syncErr := resyncNonce(masterAuthority); syncErr == nil {
			result = attemptCreateToken(name, symbol, decimals, masterAuthority)
		}
	}
	return result
}

func attemptCreateToken(name, symbol string, decimals int32, masterAuthority string) *ResponseHandler[*TokenIssueResult] {
	blockNum, err := getBlockNumber()
	if err != nil {
		return &ResponseHandler[*TokenIssueResult]{err: err}
	}

	userNonce, err := allocateNonce(masterAuthority)
	if err != nil {
		return &ResponseHandler[*TokenIssueResult]{err: err}
	}

	// Signing params — all fields of IssueParams except Signature, sorted a-z by key.
	// nonce is kept at 0 (server ignores tx nonce suggestions but still verifies the field).
	signingParams := map[string]interface{}{
		"chainId":          chainID,
		"decimals":         decimals,
		"masterAuthority":  masterAuthority,
		"name":             name,
		"nonce":            int64(0),
		"recentCheckpoint": blockNum,
		"symbol":           symbol,
		"userNonce":        userNonce,
	}

	signature, err := generateSignature(signingParams)
	if err != nil {
		return &ResponseHandler[*TokenIssueResult]{err: err}
	}

	reqParams := map[string]interface{}{
		"chainId":          chainID,
		"decimals":         decimals,
		"masterAuthority":  masterAuthority,
		"name":             name,
		"nonce":            int64(0),
		"recentCheckpoint": blockNum,
		"symbol":           symbol,
		"userNonce":        userNonce,
		"signature": map[string]string{
			"r": signature.R,
			"s": signature.S,
			"v": signature.V,
		},
	}

	raw, err := rpcCall("create_token", reqParams)
	if err != nil {
		return &ResponseHandler[*TokenIssueResult]{err: err}
	}

	var response TokenIssueResult
	if err := json.Unmarshal(raw, &response); err != nil {
		return &ResponseHandler[*TokenIssueResult]{err: err}
	}

	return &ResponseHandler[*TokenIssueResult]{data: &response}
}

// GetTokenMetadata gets token metadata
func GetTokenMetadata(tokenAddress string) *ResponseHandler[*TokenMetadata] {
	return dynamicCallWithType[*TokenMetadata](tokenAddress, "getTokenMetadata", []interface{}{})
}

// UpdateMetadata updates token metadata
func UpdateMetadata(tokenAddress, newName, newSymbol string) *ResponseHandler[*TransactionResult] {
	return dynamicCall(tokenAddress, "updateMetadata", []interface{}{newName, newSymbol})
}

// Mint mints new tokens
func Mint(tokenAddress, toAddress, amount string) *ResponseHandler[*TransactionResult] {
	return dynamicCall(tokenAddress, "mint", []interface{}{toAddress, amount})
}

// GrantAuthority grants authority to account
func GrantAuthority(tokenAddress, role, account string) *ResponseHandler[*TransactionResult] {
	return dynamicCall(tokenAddress, "grantAuthority", []interface{}{role, account})
}

// RevokeAuthority revokes authority from account
func RevokeAuthority(tokenAddress, role, account string) *ResponseHandler[*TransactionResult] {
	return dynamicCall(tokenAddress, "revokeAuthority", []interface{}{role, account})
}

// AdminBurn burns tokens by admin
func AdminBurn(tokenAddress, fromAddress, amount string) *ResponseHandler[*TransactionResult] {
	return dynamicCall(tokenAddress, "adminBurn", []interface{}{fromAddress, amount})
}

// Pause pauses the contract
func Pause(tokenAddress string) *ResponseHandler[*TransactionResult] {
	return dynamicCall(tokenAddress, "pause", []interface{}{})
}

// Unpause unpauses the contract
func Unpause(tokenAddress string) *ResponseHandler[*TransactionResult] {
	return dynamicCall(tokenAddress, "unpause", []interface{}{})
}

// AddToBlacklist adds account to blacklist
func AddToBlacklist(tokenAddress, accountAddress string) *ResponseHandler[*TransactionResult] {
	return dynamicCall(tokenAddress, "addToBlacklist", []interface{}{accountAddress})
}

// BalanceInfo contains balance information
type BalanceInfo struct {
	Wei string `json:"wei"`
	Eth string `json:"eth"`
}

// GetBalance gets account ACH balance
func GetBalance(address string) *ResponseHandler[*BalanceInfo] {
	rpcReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_getBalance",
		"params":  []interface{}{address, "latest"},
		"id":      1,
	}

	reqBody, _ := json.Marshal(rpcReq)
	resp, err := client.Post(baseURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return &ResponseHandler[*BalanceInfo]{err: err}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var rpcResp struct {
		Result string `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	json.Unmarshal(respBody, &rpcResp)

	if rpcResp.Error != nil {
		return &ResponseHandler[*BalanceInfo]{err: fmt.Errorf("RPC error: %s", rpcResp.Error.Message)}
	}

	balanceHex := rpcResp.Result
	balanceWei := new(big.Int)
	balanceWei.SetString(balanceHex[2:], 16)

	ethValue := new(big.Float).SetInt(balanceWei)
	ethValue.Quo(ethValue, big.NewFloat(1e18))

	return &ResponseHandler[*BalanceInfo]{data: &BalanceInfo{
		Wei: balanceWei.String(),
		Eth: ethValue.String(),
	}}
}

// TokenBalanceInfo contains ERC-20 token balance information
type TokenBalanceInfo struct {
	Amount string `json:"amount"` // raw token amount (unscaled, smallest unit)
}

// GetTokenBalance queries the ERC-20 token balance of a wallet address.
func GetTokenBalance(tokenAddress, walletAddress string) *ResponseHandler[*TokenBalanceInfo] {
	// balanceOf(address) selector = keccak256("balanceOf(address)")[:4] = 0x70a08231
	// ABI-encode the wallet address: left-pad to 32 bytes
	addr := walletAddress
	if len(addr) >= 2 && addr[:2] == "0x" {
		addr = addr[2:]
	}
	callData := fmt.Sprintf("0x70a08231%064s", addr)

	rpcReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_call",
		"params": []interface{}{
			map[string]string{
				"to":   tokenAddress,
				"data": callData,
			},
			"latest",
		},
		"id": 1,
	}

	reqBody, _ := json.Marshal(rpcReq)
	resp, err := client.Post(baseURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return &ResponseHandler[*TokenBalanceInfo]{err: err}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var rpcResp struct {
		Result string `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	json.Unmarshal(respBody, &rpcResp)

	if rpcResp.Error != nil {
		return &ResponseHandler[*TokenBalanceInfo]{err: fmt.Errorf("RPC error: %s", rpcResp.Error.Message)}
	}

	hex := rpcResp.Result
	if len(hex) >= 2 && hex[:2] == "0x" {
		hex = hex[2:]
	}
	amount := new(big.Int)
	amount.SetString(hex, 16)

	return &ResponseHandler[*TokenBalanceInfo]{data: &TokenBalanceInfo{
		Amount: amount.String(),
	}}
}

// dynamicCallWithType is a generic dynamic call supporting different return types.
// userNonce 由本地缓存自动分配，遇到 nonce 不匹配时重同步后自动重试一次。
func dynamicCallWithType[T any](tokenAddress, methodName string, methodArgs []interface{}) *ResponseHandler[T] {
	callerAddr, err := getCallerAddress()
	if err != nil {
		return &ResponseHandler[T]{err: err}
	}

	result := attemptDynamicCall[T](tokenAddress, methodName, methodArgs, callerAddr)
	if isNonceMismatch(result.err) {
		if syncErr := resyncNonce(callerAddr); syncErr == nil {
			result = attemptDynamicCall[T](tokenAddress, methodName, methodArgs, callerAddr)
		}
	}
	return result
}

func attemptDynamicCall[T any](tokenAddress, methodName string, methodArgs []interface{}, callerAddr string) *ResponseHandler[T] {
	blockNum, err := getBlockNumber()
	if err != nil {
		return &ResponseHandler[T]{err: err}
	}

	userNonce, err := allocateNonce(callerAddr)
	if err != nil {
		return &ResponseHandler[T]{err: err}
	}

	// Signing params — all fields of DynamicCallParams except Signature, sorted a-z by key.
	// methodName now participates in signing (Finding 3) to prevent cross-method replay.
	// nonce is kept at 0 (server ignores tx nonce suggestions).
	signingParams := map[string]interface{}{
		"chainId":          chainID,
		"methodArgs":       methodArgs,
		"methodName":       methodName,
		"nonce":            int64(0),
		"recentCheckpoint": blockNum,
		"token":            tokenAddress,
		"userNonce":        userNonce,
	}

	signature, err := generateSignature(signingParams)
	if err != nil {
		return &ResponseHandler[T]{err: err}
	}

	reqParams := map[string]interface{}{
		"chainId":          chainID,
		"methodArgs":       methodArgs,
		"methodName":       methodName,
		"nonce":            int64(0),
		"recentCheckpoint": blockNum,
		"token":            tokenAddress,
		"userNonce":        userNonce,
		"signature": map[string]string{
			"r": signature.R,
			"s": signature.S,
			"v": signature.V,
		},
	}

	raw, err := rpcCall(methodName, reqParams)
	if err != nil {
		return &ResponseHandler[T]{err: err}
	}

	var response T
	if err := json.Unmarshal(raw, &response); err != nil {
		return &ResponseHandler[T]{err: err}
	}

	return &ResponseHandler[T]{data: response}
}

// dynamicCall is a wrapper returning TransactionResult
func dynamicCall(tokenAddress, methodName string, methodArgs []interface{}) *ResponseHandler[*TransactionResult] {
	return dynamicCallWithType[*TransactionResult](tokenAddress, methodName, methodArgs)
}

// rpcCall performs a JSON-RPC call to the token RPC endpoint
func rpcCall(method string, params interface{}) (json.RawMessage, error) {
	rpcReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      1,
	}

	reqBody, _ := json.Marshal(rpcReq)
	resp, err := client.Post(baseURL+"/rpc", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	json.Unmarshal(respBody, &rpcResp)

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error: %s", rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

// getBlockNumber returns the current block number for use as recentCheckpoint
func getBlockNumber() (int64, error) {
	resp, err := client.Post(baseURL, "application/json",
		bytes.NewBufferString(`{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}`))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var blockResp struct {
		Result string `json:"result"`
	}

	json.Unmarshal(respBody, &blockResp)

	var blockNumber int64
	fmt.Sscanf(blockResp.Result, "0x%x", &blockNumber)

	return blockNumber, nil
}
