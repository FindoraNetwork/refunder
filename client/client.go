package client

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/FindoraNetwork/refunder/config"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Client is a wrapper of ethclient (normal usage) and simulated backend (test usage)
// The goal is providing a simple way for services can do reconnecting
type Client interface {
	Dial() (Client, error)
	NetworkID(context.Context) (*big.Int, error)
	Close()
	ethereum.LogFilterer
	ethereum.TransactionSender
	ethereum.ChainStateReader
	ethereum.PendingStateReader
	ethereum.GasPricer
}

type client struct {
	config    *config.Server
	ethclient *ethclient.Client
}

// New returns a ethclient wrapper structure and dialed a connection with the server
func New(config *config.Server) (Client, error) {
	c := &client{
		config: config,
	}
	return c.Dial()
}

// Dial calls the ethclient.DialContext directly
func (c *client) Dial() (Client, error) {
	dialTimeout, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(c.config.ServerDialTimeoutSec)*time.Second,
	)
	defer cancel()

	// ws://prod-testnet-us-west-2-sentry-000-public.prod.findora.org:8546
	// NOTE: Findora network only works on sentry node and no TLS supported
	client, err := ethclient.DialContext(dialTimeout, c.config.ServerWSAddress)
	if err != nil {
		return nil, fmt.Errorf("ethclient.Dial failed: %w, config: %v", err, c.config)
	}

	c.ethclient = client
	return c, nil
}

// BalanceAt calls the ethclient.BalanceAt directly
func (c *client) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	return c.ethclient.BalanceAt(ctx, account, blockNumber)
}

// StorageAt calls the ethclient.StorageAt directly
func (c *client) StorageAt(ctx context.Context, account common.Address, key common.Hash, blockNumber *big.Int) ([]byte, error) {
	return c.ethclient.StorageAt(ctx, account, key, blockNumber)
}

// CodeAt calls the ethclient.CodeAt directly
func (c *client) CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error) {
	return c.ethclient.CodeAt(ctx, account, blockNumber)
}

// NonceAt calls the ethclient.NonceAt directly
func (c *client) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	return c.ethclient.NonceAt(ctx, account, blockNumber)
}

// NetworkID calls the ethclient.NetworkID directly
func (c *client) NetworkID(ctx context.Context) (*big.Int, error) {
	return c.ethclient.NetworkID(ctx)
}

// Close calls the ethclient.Close directly
func (c *client) Close() {
	c.ethclient.Close()
}

// FilterLogs calls the ethclient.FilterLogs directly
func (c *client) FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	return c.ethclient.FilterLogs(ctx, q)
}

// SubscribeFilterLogs calls the ethclient.SubscribeFilterLogs directly
func (c *client) SubscribeFilterLogs(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	return c.ethclient.SubscribeFilterLogs(ctx, q, ch)
}

// SendTransaction calls the ethclient.SendTransaction directly
func (c *client) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return c.ethclient.SendTransaction(ctx, tx)
}

// PendingBalanceAt calls the ethclient.PendingBalanceAt directly
func (c *client) PendingBalanceAt(ctx context.Context, account common.Address) (*big.Int, error) {
	return c.ethclient.PendingBalanceAt(ctx, account)
}

// PendingStorageAt calls the ethclient.PendingStorageAt directly
func (c *client) PendingStorageAt(ctx context.Context, account common.Address, key common.Hash) ([]byte, error) {
	return c.ethclient.PendingStorageAt(ctx, account, key)
}

// PendingCodeAt calls the ethclient.PendingCodeAt directly
func (c *client) PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error) {
	return c.ethclient.PendingCodeAt(ctx, account)
}

// PendingNonceAt calls the ethclient.PendingNonceAt directly
func (c *client) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return c.ethclient.PendingNonceAt(ctx, account)
}

// PendingTransactionCount calls the ethclient.PendingTransactionCount directly
func (c *client) PendingTransactionCount(ctx context.Context) (uint, error) {
	return c.ethclient.PendingTransactionCount(ctx)
}

// SuggestGasPrice calls the ethclient.SuggestGasPrice directly
func (c *client) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return c.ethclient.SuggestGasPrice(ctx)
}
