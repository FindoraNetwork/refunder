package gasfee_test

import (
	"context"
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/FindoraNetwork/refunder/client"
	"github.com/FindoraNetwork/refunder/config"
	"github.com/FindoraNetwork/refunder/gasfee"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/backends"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
)

type mockClient struct {
	client *backends.SimulatedBackend
}

func (c *mockClient) Dial() (client.Client, error) {
	return c, nil
}
func (c *mockClient) NetworkID(ctx context.Context) (*big.Int, error) { return nil, nil }
func (c *mockClient) Close()                                          {}
func (c *mockClient) FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	return nil, nil
}

func (c *mockClient) SubscribeFilterLogs(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	return c.client.SubscribeFilterLogs(ctx, q, ch)
}

func (c *mockClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return c.client.SendTransaction(ctx, tx)
}

func (c *mockClient) PendingBalanceAt(ctx context.Context, account common.Address) (*big.Int, error) {
	return nil, nil
}

func (c *mockClient) PendingStorageAt(ctx context.Context, account common.Address, key common.Hash) ([]byte, error) {
	return nil, nil
}

func (c *mockClient) PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error) {
	return nil, nil
}

func (c *mockClient) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return c.client.PendingNonceAt(ctx, account)
}
func (c *mockClient) PendingTransactionCount(ctx context.Context) (uint, error) { return 0, nil }
func (c *mockClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return c.client.SuggestGasPrice(ctx)
}

func setup(t *testing.T) (client.Client, string) {
	// https://goethereumbook.org/client-simulated/
	priv, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	auth := bind.NewKeyedTransactor(priv)
	balance := new(big.Int)
	balance.SetString("10000000000000000000", 10) // 10 token in wei
	address := auth.From
	genesisAlloc := map[common.Address]core.GenesisAccount{
		address: {
			Balance: balance,
		},
	}
	blockGasLimit := uint64(4712388)
	return &mockClient{
		client: backends.NewSimulatedBackend(genesisAlloc, blockGasLimit),
	}, hex.EncodeToString(math.PaddedBigBytes(priv.D, priv.Params().BitSize/8))
}

// TODO: waiting for the PM to confirm the requirements then
// 1. adding more detail unit tests
// 2. adding a e2e test
func Test_GaseService(t *testing.T) {
	client, privateKey := setup(t)
	service, err := gasfee.New(client, &config.GasFeeService{
		PrivateKey:                  privateKey,
		HandlerOperationsTimeoutSec: 3,
		SubscripTimeoutSec:          3,
		EventLogPoolSize:            3,
		WorkerPoolSize:              3,
		WorkerPoolWorkerNum:         3,
		RefundAmounts: map[string]*config.GasFeeRefundAmount{
			"0x49C86Ee3Aca6ADE64127FA170445cd0B97CBBd4c": {
				Threshold: 3,
				Refund:    1,
			},
		},
	},
	)
	assert.NoError(t, err)
	assert.NoError(t, service.Start())
	service.Close()
}
