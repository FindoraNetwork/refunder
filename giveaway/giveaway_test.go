package giveaway_test

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/FindoraNetwork/refunder/client"
	"github.com/FindoraNetwork/refunder/config"
	"github.com/FindoraNetwork/refunder/giveaway"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/backends"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
)

func setup(t *testing.T) (client.Client, string) {
	// https://goethereumbook.org/client-simulated/
	priv, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(priv, big.NewInt(1337))
	if err != nil {
		t.Fatal(err)
	}
	balance := new(big.Int)
	balance.SetString("10000000000000000000", 10) // 10 token in wei
	address := auth.From
	genesisAlloc := map[common.Address]core.GenesisAccount{
		address: {
			Balance: balance,
		},
	}
	blockGasLimit := uint64(4712388)
	return &client.MockClient{
		Client: backends.NewSimulatedBackend(genesisAlloc, blockGasLimit),
	}, hex.EncodeToString(math.PaddedBigBytes(priv.D, priv.Params().BitSize/8))
}

func Test_GiveawayService(t *testing.T) {
	client, privateKey := setup(t)
	service, err := giveaway.New(client, &config.GiveawayService{
		PrivateKey:             privateKey,
		HandlerTotalTimeoutSec: 3,
		SubscripTimeoutSec:     3,
		EventLogPoolSize:       3,
		TokenAddresses:         []string{"0x49C86Ee3Aca6ADE64127FA170445cd0B97CBBd4c"},
	},
	)
	assert.NoError(t, err)
	assert.NoError(t, service.Start())
	service.Close()
}
