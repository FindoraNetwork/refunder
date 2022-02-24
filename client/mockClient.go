package client

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/backends"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type MockClient struct {
	Client *backends.SimulatedBackend
}

func (c *MockClient) DialRPC() (Client, error) {
	return c, nil
}

func (c *MockClient) DialWS() (Client, error) {
	return c, nil
}

func (c *MockClient) TransactionByHash(ctx context.Context, txHash common.Hash) (tx *types.Transaction, isPending bool, err error) {
	return
}

func (c *MockClient) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	return nil, nil
}

func (c *MockClient) BlockNumber(context.Context) (uint64, error) {
	return 0, nil
}

func (c *MockClient) NetworkID(ctx context.Context) (*big.Int, error) {
	return nil, nil
}

func (c *MockClient) Close() {
}

func (c *MockClient) FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	return c.Client.FilterLogs(ctx, q)
}

func (c *MockClient) SubscribeFilterLogs(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	return c.Client.SubscribeFilterLogs(ctx, q, ch)
}

func (c *MockClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return c.Client.SendTransaction(ctx, tx)
}

func (c *MockClient) PendingBalanceAt(ctx context.Context, account common.Address) (*big.Int, error) {
	return nil, nil
}

func (c *MockClient) PendingStorageAt(ctx context.Context, account common.Address, key common.Hash) ([]byte, error) {
	return nil, nil
}

func (c *MockClient) PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error) {
	return c.Client.PendingCodeAt(ctx, account)
}

func (c *MockClient) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return c.Client.PendingNonceAt(ctx, account)
}

func (c *MockClient) PendingTransactionCount(ctx context.Context) (uint, error) {
	return 0, nil
}

func (c *MockClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return c.Client.SuggestGasPrice(ctx)
}

func (c *MockClient) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	return c.Client.BalanceAt(ctx, account, blockNumber)
}

func (c *MockClient) StorageAt(ctx context.Context, account common.Address, key common.Hash, blockNumber *big.Int) ([]byte, error) {
	return c.Client.StorageAt(ctx, account, key, blockNumber)
}

func (c *MockClient) CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error) {
	return c.Client.CodeAt(ctx, account, blockNumber)
}

func (c *MockClient) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	return c.Client.NonceAt(ctx, account, blockNumber)
}

func (c *MockClient) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	return c.Client.BlockByHash(ctx, hash)
}

func (c *MockClient) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	return c.Client.BlockByNumber(ctx, number)
}

func (c *MockClient) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	return c.Client.HeaderByHash(ctx, hash)
}

func (c *MockClient) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	return c.Client.HeaderByNumber(ctx, number)
}

func (c *MockClient) TransactionCount(ctx context.Context, blockHash common.Hash) (uint, error) {
	return c.Client.TransactionCount(ctx, blockHash)
}

func (c *MockClient) TransactionInBlock(ctx context.Context, blockHash common.Hash, index uint) (*types.Transaction, error) {
	return c.Client.TransactionInBlock(ctx, blockHash, index)
}

func (c *MockClient) SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error) {
	return c.Client.SubscribeNewHead(ctx, ch)
}
