//go:build e2e
// +build e2e

package e2e_test

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/FindoraNetwork/refunder/client"
	"github.com/FindoraNetwork/refunder/config"
	"github.com/FindoraNetwork/refunder/e2e/giveaway/contract"
	"github.com/FindoraNetwork/refunder/giveaway"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/suite"
)

type giveawayTestSuite struct {
	suite.Suite

	chainID    *big.Int
	instance   *contract.Contract
	privateKey *ecdsa.PrivateKey
	fromAddr   common.Address
	tokenAddr  common.Address
	toAddrs    []common.Address
	serv       *giveaway.Service
}

func TestE2EGiveawayTestSuite(t *testing.T) {
	suite.Run(t, &giveawayTestSuite{
		toAddrs: make([]common.Address, 0, 3),
		chainID: big.NewInt(2153),
	})
}

func (s *giveawayTestSuite) SetupSuite() {
	// Step 1. deploy a token creation contract to do the transfer action
	s.setupSuiteDeployContract()
	// Step 2. generate three destination wallets
	s.setupSuiteGenerateWallets()
	// Step 3. start giveaway service
	s.setupSuiteStartService()
}

func (s *giveawayTestSuite) setupSuiteStartService() {
	c, err := client.New(&config.Server{
		ServerDialTimeoutSec: 3,
		ServerWSAddress:      "ws://prod-testnet-us-west-2-sentry-003-public.prod.findora.org:8546",
	})
	s.Require().NoError(err, "client.New")

	srv, err := giveaway.New(c, &config.GiveawayService{
		PrivateKey:             hexutil.Encode(crypto.FromECDSA(s.privateKey)[2:]),
		HandlerTotalTimeoutSec: 3,
		SubscripTimeoutSec:     1,
		EventLogPoolSize:       9,
		FixedGiveawayWei:       big.NewInt(30000000000000000), // 0.003
		MaxCapWei:              big.NewInt(60000000000000000), // 0.006
		TokenAddresses:         []string{s.tokenAddr.String()},
	})
	s.Require().NoError(err, "giveaway.New")

	s.serv = srv
}

func (s *giveawayTestSuite) setupSuiteGenerateWallets() {
	for i := 0; i < 3; i++ {
		privateKey, err := crypto.GenerateKey()
		s.Require().NoError(err, crypto.GenerateKey)
		publicKey, ok := privateKey.Public().(*ecdsa.PublicKey)
		s.Require().True(ok, "privateKey.Public().(*ecdsa.PublicKey)")
		s.toAddrs = append(s.toAddrs, crypto.PubkeyToAddress(*publicKey))
	}
}

func (s *giveawayTestSuite) setupSuiteDeployContract() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c, err := ethclient.DialContext(ctx, "ws://prod-testnet-us-west-2-sentry-003-public.prod.findora.org:8546")
	s.Require().NoError(err, "ethclient.DialContext")

	privateKey, err := crypto.HexToECDSA(os.Getenv("TESTING_WALLET_PRIVATE_KEY"))
	s.Require().NoError(err, "crypto.HexToECDSA")

	publicKey, ok := privateKey.Public().(*ecdsa.PublicKey)
	s.Require().True(ok, "privateKey.Public().(*ecdsa.PublicKey)")

	fromAddr := crypto.PubkeyToAddress(*publicKey)
	nonce, err := c.PendingNonceAt(ctx, fromAddr)
	s.Require().NoError(err, "c.PendingNonceAt")

	gasPrice, err := c.SuggestGasPrice(ctx)
	s.Require().NoError(err, "c.SuggestGasPrice")

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, s.chainID)
	s.Require().NoError(err, "bind.NewKeyedTransactorWithChainID")

	auth.Nonce = big.NewInt(0).SetUint64(nonce + 1)
	auth.Value = big.NewInt(0)
	auth.GasLimit = 300000
	auth.GasPrice = gasPrice

	addr, tx, instance, err := contract.DeployContract(auth, c)
	s.Require().NoError(err, "contract.DeployContract")

	s.T().Logf("giveawayTestSuite, address: %v", addr)
	s.T().Logf("giveawayTestSuite, tx: %v", tx)
	s.instance = instance
	s.privateKey = privateKey
	s.tokenAddr = addr
	s.fromAddr = fromAddr
}

func (s *giveawayTestSuite) TearDownSuite() {
	s.serv.Close()
}

func (s *giveawayTestSuite) Test_E2E_Giveaway() {
	// total timeout for this test case
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c, err := ethclient.DialContext(ctx, "ws://prod-testnet-us-west-2-sentry-003-public.prod.findora.org:8546")
	s.Require().NoError(err, "ethclient.DialContext")

	for i := 0; i < len(s.toAddrs)-1; i++ {
		toAddr := s.toAddrs[i]
		gasPrice, err := c.SuggestGasPrice(ctx)
		s.Require().NoError(err, "c.SuggestGasPrice")
		nonce, err := c.PendingNonceAt(ctx, s.fromAddr)
		s.Require().NoError(err, "c.PendingNonceAt")

		auth, err := bind.NewKeyedTransactorWithChainID(s.privateKey, s.chainID)
		s.Require().NoError(err, "bind.NewKeyedTransactorWithChainID")

		auth.Nonce = big.NewInt(0).SetUint64(nonce + 1)
		auth.Value = big.NewInt(0)
		auth.GasLimit = 300000
		auth.GasPrice = gasPrice

		tx, err := s.instance.Mint(auth, toAddr, big.NewInt(90000000000000000))
		s.Require().NoError(err, "instance.Mint")
		s.T().Logf("mint, toAddr:%v, tx:%v", toAddr, tx)

		time.Sleep(time.Second)

		balance, err := s.instance.BalanceOf(&bind.CallOpts{
			From:    s.fromAddr,
			Context: ctx,
		}, toAddr)
		s.Require().NoError(err, "instance.BalanceOf")
		s.Require().Equal(balance, big.NewInt(30000000000000000))
	}

	// the last recipient should not receive the incentive because MaxCapWei is 0.006
	balance, err := s.instance.BalanceOf(&bind.CallOpts{
		From:    s.fromAddr,
		Context: ctx,
	}, s.toAddrs[len(s.toAddrs)-1])
	s.Require().NoError(err, "instance.BalanceOf")
	s.Require().Equal(balance, big.NewInt(0))
}
