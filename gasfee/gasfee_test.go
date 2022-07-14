package gasfee_test

import (
	"encoding/binary"
	"encoding/hex"
	"io/ioutil"
	"math/big"
	"testing"
	"time"

	"github.com/FindoraNetwork/refunder/client"
	"github.com/FindoraNetwork/refunder/config"
	"github.com/FindoraNetwork/refunder/gasfee"
	"github.com/stretchr/testify/assert"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/backends"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/crypto"
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

func Test_GasfeeService(t *testing.T) {
	tmpCurBlock, err := ioutil.TempFile("", "gasfee_e2e_current_block_file_*")
	assert.NoErrorf(t, err, "ioutil.TempFile:%v", err)

	wantBlockNum := uint64(1000)

	client, privateKey := setup(t)
	service, err := gasfee.New(client, &config.GasfeeService{
		PrivateKey:                 privateKey,
		CrawleInEveryMinutes:       3,
		RefundEveryDayAt:           time.Now().UTC().Add(3 * time.Second),
		RefunderTotalTimeoutSec:    3,
		CrawlerTotalTimeoutSec:     3,
		RefundThreshold:            big.NewFloat(3),
		RefunderStartBlockNumber:   wantBlockNum,
		CurrentBlockNumberFilepath: tmpCurBlock.Name(),
	})
	assert.NoError(t, err)
	service.Close()

	curBlockNumB, err := ioutil.ReadFile(tmpCurBlock.Name())
	assert.NoError(t, err)

	gotBlockNum, _ := binary.Uvarint(curBlockNumB)
	assert.Equal(t, wantBlockNum, gotBlockNum)
}
