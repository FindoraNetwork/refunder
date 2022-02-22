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

func (c *MockClient) Dial() (Client, error) {
	return c, nil
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
