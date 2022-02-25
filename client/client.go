package client

import (
	"context"
	"errors"
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
	DialWS() (Client, error)
	DialRPC() (Client, error)
	NetworkID(context.Context) (*big.Int, error)
	Close()
	BlockNumber(context.Context) (uint64, error)
	ethereum.LogFilterer
	ethereum.TransactionSender
	ethereum.TransactionReader
	ethereum.ChainStateReader
	ethereum.PendingStateReader
	ethereum.GasPricer
	ethereum.ChainReader
}

type client struct {
	config      *config.Server
	rpcclient   *ethclient.Client
	wsclient    *ethclient.Client
	retryTimes  int
	retryPeriod time.Duration
}

// New returns a ethclient wrapper structure and dialed a connection with the server
func New(config *config.Server) Client {
	return &client{
		config:      config,
		retryTimes:  3,
		retryPeriod: time.Microsecond,
	}
}

// DialRPC calls the ethclient.DialContext directly with http address
func (c *client) DialRPC() (Client, error) {
	dialTimeout, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(c.config.ServerDialTimeoutSec)*time.Second,
	)
	defer cancel()

	client, err := ethclient.DialContext(dialTimeout, c.config.ServerRPCAddress)
	if err != nil {
		return nil, fmt.Errorf("ethclient.Dial failed: %w, config: %v", err, c.config)
	}

	c.rpcclient = client
	return c, nil
}

// DialWS calls the ethclient.DialContext directly with websocket address
func (c *client) DialWS() (Client, error) {
	dialTimeout, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(c.config.ServerDialTimeoutSec)*time.Second,
	)
	defer cancel()

	client, err := ethclient.DialContext(dialTimeout, c.config.ServerWSAddress)
	if err != nil {
		return nil, fmt.Errorf("ethclient.Dial failed: %w, config: %v", err, c.config)
	}

	c.wsclient = client
	return c, nil
}

func (c *client) TransactionByHash(ctx context.Context, txHash common.Hash) (tx *types.Transaction, isPending bool, err error) {
	if c.rpcclient == nil {
		return nil, true, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		tx, isPending, err = c.rpcclient.TransactionByHash(ctx, txHash)
		if err == nil {
			return
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

func (c *client) TransactionReceipt(ctx context.Context, txHash common.Hash) (v *types.Receipt, err error) {
	if c.rpcclient == nil {
		return nil, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.TransactionReceipt(ctx, txHash)
		if err == nil {
			return
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// BlockNumber calls the ethclient.BlockNumber directly
func (c *client) BlockNumber(ctx context.Context) (v uint64, err error) {
	if c.rpcclient == nil {
		return 0, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.BlockNumber(ctx)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

var ErrDialFirst = errors.New("client instance is nil, call dial function first")

// BlockByHash calls the ethclient.BlockByHash directly
func (c *client) BlockByHash(ctx context.Context, hash common.Hash) (v *types.Block, err error) {
	if c.rpcclient == nil {
		return nil, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.BlockByHash(ctx, hash)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// BlockByNumber calls the ethclient.BlockByNumber directly
func (c *client) BlockByNumber(ctx context.Context, number *big.Int) (v *types.Block, err error) {
	if c.rpcclient == nil {
		return nil, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.BlockByNumber(ctx, number)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// HeaderByHash calls the ethclient.HeaderByHash directly
func (c *client) HeaderByHash(ctx context.Context, hash common.Hash) (v *types.Header, err error) {
	if c.rpcclient == nil {
		return nil, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.HeaderByHash(ctx, hash)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// HeaderByNumber calls the ethclient.HeaderByNumber directly
func (c *client) HeaderByNumber(ctx context.Context, number *big.Int) (v *types.Header, err error) {
	if c.rpcclient == nil {
		return nil, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.HeaderByNumber(ctx, number)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// TransactionCount calls the ethclient.TransactionCount directly
func (c *client) TransactionCount(ctx context.Context, blockHash common.Hash) (v uint, err error) {
	if c.rpcclient == nil {
		return 0, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.TransactionCount(ctx, blockHash)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// TransactionInBlock calls the ethclient.TransactionInBlock directly
func (c *client) TransactionInBlock(ctx context.Context, blockHash common.Hash, index uint) (v *types.Transaction, err error) {
	if c.rpcclient == nil {
		return nil, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.TransactionInBlock(ctx, blockHash, index)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// SubscribeNewHead calls the ethclient.SubscribeNewHead directly
func (c *client) SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (v ethereum.Subscription, err error) {
	if c.wsclient == nil {
		return nil, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.wsclient.SubscribeNewHead(ctx, ch)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// BalanceAt calls the ethclient.BalanceAt directly
func (c *client) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (v *big.Int, err error) {
	if c.rpcclient == nil {
		return nil, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.BalanceAt(ctx, account, blockNumber)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// StorageAt calls the ethclient.StorageAt directly
func (c *client) StorageAt(ctx context.Context, account common.Address, key common.Hash, blockNumber *big.Int) (v []byte, err error) {
	if c.rpcclient == nil {
		return nil, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.StorageAt(ctx, account, key, blockNumber)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// CodeAt calls the ethclient.CodeAt directly
func (c *client) CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) (v []byte, err error) {
	if c.rpcclient == nil {
		return nil, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.CodeAt(ctx, account, blockNumber)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// NonceAt calls the ethclient.NonceAt directly
func (c *client) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (v uint64, err error) {
	if c.rpcclient == nil {
		return 0, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.NonceAt(ctx, account, blockNumber)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// NetworkID calls the ethclient.NetworkID directly
func (c *client) NetworkID(ctx context.Context) (v *big.Int, err error) {
	if c.rpcclient == nil {
		return nil, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.NetworkID(ctx)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// Close calls the ethclient.Close directly
func (c *client) Close() {
	if c.rpcclient != nil {
		c.rpcclient.Close()
	}
	if c.wsclient != nil {
		c.wsclient.Close()
	}
}

// FilterLogs calls the ethclient.FilterLogs directly
func (c *client) FilterLogs(ctx context.Context, q ethereum.FilterQuery) (v []types.Log, err error) {
	if c.rpcclient == nil {
		return nil, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.FilterLogs(ctx, q)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// SubscribeFilterLogs calls the ethclient.SubscribeFilterLogs directly
func (c *client) SubscribeFilterLogs(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (v ethereum.Subscription, err error) {
	if c.wsclient == nil {
		return nil, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.wsclient.SubscribeFilterLogs(ctx, q, ch)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// SendTransaction calls the ethclient.SendTransaction directly
func (c *client) SendTransaction(ctx context.Context, tx *types.Transaction) (err error) {
	if c.rpcclient == nil {
		return ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		err = c.rpcclient.SendTransaction(ctx, tx)
		if err == nil {
			return nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// PendingBalanceAt calls the ethclient.PendingBalanceAt directly
func (c *client) PendingBalanceAt(ctx context.Context, account common.Address) (v *big.Int, err error) {
	if c.rpcclient == nil {
		return nil, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.PendingBalanceAt(ctx, account)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// PendingStorageAt calls the ethclient.PendingStorageAt directly
func (c *client) PendingStorageAt(ctx context.Context, account common.Address, key common.Hash) (v []byte, err error) {
	if c.rpcclient == nil {
		return nil, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.PendingStorageAt(ctx, account, key)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// PendingCodeAt calls the ethclient.PendingCodeAt directly
func (c *client) PendingCodeAt(ctx context.Context, account common.Address) (v []byte, err error) {
	if c.rpcclient == nil {
		return nil, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.PendingCodeAt(ctx, account)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// PendingNonceAt calls the ethclient.PendingNonceAt directly
func (c *client) PendingNonceAt(ctx context.Context, account common.Address) (v uint64, err error) {
	if c.rpcclient == nil {
		return 0, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.PendingNonceAt(ctx, account)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// PendingTransactionCount calls the ethclient.PendingTransactionCount directly
func (c *client) PendingTransactionCount(ctx context.Context) (v uint, err error) {
	if c.rpcclient == nil {
		return 0, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.PendingTransactionCount(ctx)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}

// SuggestGasPrice calls the ethclient.SuggestGasPrice directly
func (c *client) SuggestGasPrice(ctx context.Context) (v *big.Int, err error) {
	if c.rpcclient == nil {
		return nil, ErrDialFirst
	}
	for i := 0; i < c.retryTimes; i++ {
		v, err = c.rpcclient.SuggestGasPrice(ctx)
		if err == nil {
			return v, nil
		}
		time.Sleep(c.retryPeriod)
	}
	return
}
