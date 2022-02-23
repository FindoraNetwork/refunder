//go:build e2e
// +build e2e

package e2e_test

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FindoraNetwork/refunder/client"
	"github.com/FindoraNetwork/refunder/config"
	"github.com/FindoraNetwork/refunder/e2e/giveaway/contract"
	"github.com/FindoraNetwork/refunder/giveaway"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/suite"
)

type giveawayTestSuite struct {
	suite.Suite

	chainID          *big.Int
	instance         *contract.Contract
	privateKey       *ecdsa.PrivateKey
	fromAddr         common.Address
	tokenAddr        common.Address
	toAddrs          []common.Address
	serv             *giveaway.Service
	evmWSAddress     string
	evmPRCAddress    string
	blockTime        time.Duration
	fixedGiveawayWei *big.Int
	maxCapWei        *big.Int
}

func TestE2EGiveawayTestSuite(t *testing.T) {
	suite.Run(t, &giveawayTestSuite{
		toAddrs: make([]common.Address, 0, 3),
		// chainID:          big.NewInt(2153),
		// evmWSAddress:     "ws://prod-testnet-us-west-2-sentry-003-public.prod.findora.org:8546",
		// evmPRCAddress:    "http://prod-testnet-us-west-2-full-003-open.prod.findora.org:8545",
		chainID:          big.NewInt(18),
		evmWSAddress:     "wss://testnet-ws.thundercore.com",
		evmPRCAddress:    "https://testnet-rpc.thundercore.com",
		blockTime:        16 * time.Second,
		fixedGiveawayWei: big.NewInt(3000000000000000), // 0.003
		maxCapWei:        big.NewInt(6000000000000000), // 0.006
	})
}

func (s *giveawayTestSuite) SetupSuite() {
	// Step 1. deploy a token creation contract to do the transfer action
	s.setupSuiteDeployContract()
	s.T().Log("setupSuiteDeployContract done")
	// Step 2. generate three destination wallets
	s.setupSuiteGenerateWallets()
	s.T().Log("setupSuiteGenerateWallets done")
	// Step 3. start giveaway service
	s.setupSuiteStartService()
	s.T().Log("setupSuiteStartService done")
}

func (s *giveawayTestSuite) setupSuiteStartService() {
	srv, err := giveaway.New(
		client.New(&config.Server{
			ServerDialTimeoutSec: 3,
			ServerWSAddress:      s.evmWSAddress,
			ServerRPCAddress:     s.evmPRCAddress,
		}),
		&config.GiveawayService{
			PrivateKey:             strings.TrimPrefix(hexutil.Encode(crypto.FromECDSA(s.privateKey)), "0x"),
			HandlerTotalTimeoutSec: 3,
			SubscripTimeoutSec:     1,
			EventLogPoolSize:       9,
			FixedGiveawayWei:       s.fixedGiveawayWei,
			MaxCapWei:              s.maxCapWei,
			TokenAddresses:         []string{s.tokenAddr.String()},
		})
	s.Require().NoErrorf(err, "giveaway.New:%v", err)

	s.serv = srv
}

func (s *giveawayTestSuite) setupSuiteGenerateWallets() {
	for i := 0; i < 3; i++ {
		privateKey, err := crypto.GenerateKey()
		s.Require().NoErrorf(err, "crypto.GenerateKey:%v", err)
		publicKey, ok := privateKey.Public().(*ecdsa.PublicKey)
		s.Require().True(ok, "privateKey.Public().(*ecdsa.PublicKey)")
		s.toAddrs = append(s.toAddrs, crypto.PubkeyToAddress(*publicKey))
	}
}

func (s *giveawayTestSuite) setupSuiteDeployContract() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c, err := ethclient.DialContext(ctx, s.evmPRCAddress)
	s.Require().NoErrorf(err, "ethclient.DialContext:%v", err)

	privateKey, err := crypto.HexToECDSA(os.Getenv("TESTING_WALLET_PRIVATE_KEY"))
	s.Require().NoErrorf(err, "crypto.HexToECDSA:%v", err)
	s.privateKey = privateKey

	publicKey, ok := privateKey.Public().(*ecdsa.PublicKey)
	s.Require().True(ok, "privateKey.Public().(*ecdsa.PublicKey)")

	fromAddr := crypto.PubkeyToAddress(*publicKey)
	s.fromAddr = fromAddr

	addr, tx, instance, err := contract.DeployContract(s.genAuth(ctx, c), c)
	s.Require().NoErrorf(err, "contract.DeployContract:%v", err)
	s.instance = instance
	s.tokenAddr = addr

	s.T().Logf("giveawayTestSuite, address: %v", addr)
	s.T().Logf("giveawayTestSuite, tx: %v", tx)
}

func (s *giveawayTestSuite) TearDownSuite() {
	s.serv.Close()
}

func (s *giveawayTestSuite) genAuth(ctx context.Context, c *ethclient.Client) *bind.TransactOpts {
	gasPrice, err := c.SuggestGasPrice(ctx)
	s.Require().NoErrorf(err, "c.SuggestGasPrice:%v", err)

	nonce, err := c.PendingNonceAt(ctx, s.fromAddr)
	s.Require().NoErrorf(err, "c.PendingNonceAt:%v", err)

	auth, err := bind.NewKeyedTransactorWithChainID(s.privateKey, s.chainID)
	s.Require().NoErrorf(err, "bind.NewKeyedTransactorWithChainID:%v", err)

	auth.Nonce = big.NewInt(0).SetUint64(nonce)
	auth.Value = big.NewInt(0)
	auth.GasLimit = 30000000
	auth.GasPrice = gasPrice
	return auth
}

func (s *giveawayTestSuite) Test_E2E_Giveaway() {
	// total timeout for this test case
	ctx, cancel := context.WithTimeout(context.Background(), s.blockTime*time.Duration(len(s.toAddrs))*3)
	defer cancel()

	c, err := ethclient.DialContext(ctx, s.evmPRCAddress)
	s.Require().NoErrorf(err, "ethclient.DialContext:%v", err)

	type want struct {
		toAddr      common.Address
		wantBalance *big.Int
		blockNumber *big.Int
	}
	wants := make([]want, 0, len(s.toAddrs))

	// mint demo token to the receipts
	for j := 0; j < 2; j++ {
		for i := 0; i < len(s.toAddrs); i++ {
			toAddr := s.toAddrs[i]
			tx, err := s.instance.Mint(s.genAuth(ctx, c), toAddr, big.NewInt(90000000000000000))
			s.Require().NoErrorf(err, "instance.Mint:%v", err)
			s.T().Logf("mint, toAddr:%v, tx:%v", toAddr, tx)

			receipt, err := c.TransactionReceipt(ctx, tx.Hash())
		receiptLoop:
			for {
				switch err {
				case nil:
					break receiptLoop
				case ethereum.NotFound:
					// skip
				default:
					s.Require().NoErrorf(err, "c.TransactionReceipt:%v", err)
				}

				time.Sleep(time.Second)
				receipt, err = c.TransactionReceipt(ctx, tx.Hash())
			}

			// the last recipient should not receive the incentive because over the MaxCapWei
			wantBalance := s.fixedGiveawayWei
			if i == len(s.toAddrs)-1 {
				wantBalance = big.NewInt(0)
			}
			wants = append(wants, want{
				toAddr:      toAddr,
				wantBalance: wantBalance,
				blockNumber: receipt.BlockNumber,
			})
		}

		for i := 0; i < len(s.toAddrs); i++ {
			toAddr := s.toAddrs[i]
			blockNum1 := wants[i].blockNumber
			var blockNum2 uint64
			var err error
			for {
				blockNum2, err = c.BlockNumber(ctx)
				s.Require().NoErrorf(err, "c.BlockNumber:%v", err)
				if blockNum1.Uint64() < blockNum2 {
					break
				}
			}

			gotBalance, err := c.BalanceAt(ctx, toAddr, nil)
			s.Require().NoErrorf(err, "c.BalanceOf:%v", err)
			s.Require().Equalf(
				wants[i].wantBalance.Uint64(),
				gotBalance.Uint64(),
				"toAddr:%v, want:%v, got:%v",
				toAddr, wants[i].wantBalance.Uint64(), gotBalance.Uint64(),
			)
		}
	}
}
