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
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// Global configuration
var (
	baseURL    = "http://localhost:8545"
	privateKey = ""
	client     = &http.Client{Timeout: 30 * time.Second}
)

// Config configures the API endpoint and private key
func Config(url, key string) {
	baseURL = url
	privateKey = key
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

// buildSortedMessage creates message string sorted by keys a-z (consistent with server side)
func buildSortedMessage(params map[string]interface{}) string {
	// Get all keys and sort them
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys) // a-z sort

	// Build message string using sorted keys
	var messageParts []string
	for _, key := range keys {
		value := params[key]
		messageParts = append(messageParts, fmt.Sprintf("%v", value))
	}

	return strings.Join(messageParts, ",")
}

// generateSignature universal signing method - sorts keys a-z then signs
func generateSignature(params map[string]interface{}) (*Signature, error) {
	// Remove 0x prefix if present
	privateKeyHex := privateKey
	if len(privateKeyHex) > 2 && privateKeyHex[:2] == "0x" {
		privateKeyHex = privateKeyHex[2:]
	}

	// Parse private key
	privKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	// Build message string sorted by keys a-z
	message := buildSortedMessage(params)

	// Calculate message hash
	hash := crypto.Keccak256Hash([]byte(message))

	// Sign
	signature, err := crypto.Sign(hash.Bytes(), privKey)
	if err != nil {
		return nil, fmt.Errorf("sign error: %w", err)
	}

	// Extract r, s, v values
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:64])
	v := new(big.Int).SetBytes([]byte{signature[64] + 27}) // Add 27 is Ethereum convention

	return &Signature{
		R: r.String(),
		S: s.String(),
		V: v.String(),
	}, nil
}

// CreateToken creates a new token
func CreateToken(name, symbol string, decimals int32, masterAuthority string) *ResponseHandler[*TokenIssueResult] {
	blockNum, err := getBlockNumber()
	if err != nil {
		return &ResponseHandler[*TokenIssueResult]{err: err}
	}

	nonce := int64(0)

	// Build parameter mapping with consistent key names and sorting as server side
	params := map[string]interface{}{
		"decimals":         decimals,
		"masterAuthority":  masterAuthority,
		"name":             name,
		"nonce":            nonce,
		"recentCheckpoint": blockNum,
		"symbol":           symbol,
	}

	signature, err := generateSignature(params)
	if err != nil {
		return &ResponseHandler[*TokenIssueResult]{err: err}
	}

	reqParams := map[string]interface{}{
		"decimals":          decimals,
		"master_authority":  masterAuthority,
		"name":              name,
		"symbol":            symbol,
		"nonce":             nonce,
		"recent_checkpoint": blockNum,
		"signature": map[string]string{
			"r": signature.R,
			"s": signature.S,
			"v": signature.V,
		},
	}

	result, err := rpcCall("create_token", reqParams)
	if err != nil {
		return &ResponseHandler[*TokenIssueResult]{err: err}
	}

	var response TokenIssueResult
	if err := json.Unmarshal(result, &response); err != nil {
		return &ResponseHandler[*TokenIssueResult]{err: err}
	}

	return &ResponseHandler[*TokenIssueResult]{data: &response}
}

// GetTokenMetadata gets token metadata
func GetTokenMetadata(tokenAddress string) *ResponseHandler[*TokenMetadata] {
	return dynamicCallWithType[*TokenMetadata](tokenAddress, "getTokenMetadata", []interface{}{}, 0)
}

// UpdateMetadata updates token metadata
func UpdateMetadata(tokenAddress, newName, newSymbol string, nonce int64) *ResponseHandler[*TransactionResult] {
	return dynamicCall(tokenAddress, "updateMetadata", []interface{}{newName, newSymbol}, nonce)
}

// Mint mints new tokens
func Mint(tokenAddress, toAddress, amount string, nonce int64) *ResponseHandler[*TransactionResult] {
	return dynamicCall(tokenAddress, "mint", []interface{}{toAddress, amount}, nonce)
}

// GrantAuthority grants authority to account
func GrantAuthority(tokenAddress, role, account string, nonce int64) *ResponseHandler[*TransactionResult] {
	return dynamicCall(tokenAddress, "grantAuthority", []interface{}{role, account}, nonce)
}

// RevokeAuthority revokes authority from account
func RevokeAuthority(tokenAddress, role, account string, nonce int64) *ResponseHandler[*TransactionResult] {
	return dynamicCall(tokenAddress, "revokeAuthority", []interface{}{role, account}, nonce)
}

// AdminBurn burns tokens by admin
func AdminBurn(tokenAddress, fromAddress, amount string, nonce int64) *ResponseHandler[*TransactionResult] {
	return dynamicCall(tokenAddress, "adminBurn", []interface{}{fromAddress, amount}, nonce)
}

// Pause pauses the contract
func Pause(tokenAddress string, nonce int64) *ResponseHandler[*TransactionResult] {
	return dynamicCall(tokenAddress, "pause", []interface{}{}, nonce)
}

// Unpause unpauses the contract
func Unpause(tokenAddress string, nonce int64) *ResponseHandler[*TransactionResult] {
	return dynamicCall(tokenAddress, "unpause", []interface{}{}, nonce)
}

// AddToBlacklist adds account to blacklist
func AddToBlacklist(tokenAddress, accountAddress string, nonce int64) *ResponseHandler[*TransactionResult] {
	return dynamicCall(tokenAddress, "addToBlacklist", []interface{}{accountAddress}, nonce)
}

// BalanceInfo contains balance information
type BalanceInfo struct {
	Wei string `json:"wei"`
	Eth string `json:"eth"`
}

// GetBalance gets account ETH balance - direct call to Ethereum node
func GetBalance(address string) *ResponseHandler[*BalanceInfo] {
	// Direct call to Ethereum node, not our RPC server
	rpcReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_getBalance",
		"params":  []interface{}{address, "latest"},
		"id":      1,
	}

	reqBody, _ := json.Marshal(rpcReq)
	resp, err := client.Post(baseURL, "application/json", bytes.NewBuffer(reqBody)) // Note: baseURL not baseURL+"/rpc"
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

	// Convert hex string to big.Int
	balanceWei := new(big.Int)
	balanceWei.SetString(balanceHex[2:], 16) // Remove "0x" prefix

	// Convert to ETH (1 ETH = 10^18 Wei)
	ethValue := new(big.Float).SetInt(balanceWei)
	ethValue.Quo(ethValue, big.NewFloat(1e18))
	balanceEth := ethValue.String()

	response := &BalanceInfo{
		Wei: balanceWei.String(),
		Eth: balanceEth,
	}

	return &ResponseHandler[*BalanceInfo]{data: response}
}

// Internal method: generic dynamic call (supports different return types)
func dynamicCallWithType[T any](tokenAddress, methodName string, methodArgs []interface{}, nonce int64) *ResponseHandler[T] {
	blockNum, err := getBlockNumber()
	if err != nil {
		return &ResponseHandler[T]{err: err}
	}

	// Build parameter mapping with consistent key names and sorting as server side（methodName不参与签名）
	params := map[string]interface{}{
		"recentCheckpoint": blockNum,
		"nonce":            nonce,
		"token":            tokenAddress,
	}

	signature, err := generateSignature(params)
	if err != nil {
		return &ResponseHandler[T]{err: err}
	}

	reqParams := map[string]interface{}{
		"nonce":             nonce,
		"token":             tokenAddress,
		"methodArgs":        methodArgs,
		"recent_checkpoint": blockNum,
		"signature": map[string]string{
			"r": signature.R,
			"s": signature.S,
			"v": signature.V,
		},
	}

	result, err := rpcCall(methodName, reqParams)
	if err != nil {
		return &ResponseHandler[T]{err: err}
	}

	var response T
	if err := json.Unmarshal(result, &response); err != nil {
		return &ResponseHandler[T]{err: err}
	}

	return &ResponseHandler[T]{data: response}
}

// Internal method: dynamic call (backward compatible wrapper)
func dynamicCall(tokenAddress, methodName string, methodArgs []interface{}, nonce int64) *ResponseHandler[*TransactionResult] {
	return dynamicCallWithType[*TransactionResult](tokenAddress, methodName, methodArgs, nonce)
}

// Internal method: RPC call
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

// Internal method: get block number
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
