//go:build e2e
// +build e2e

package e2e_test

import (
	"context"
	"crypto/ecdsa"
	"io/ioutil"
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
		toAddrs:          make([]common.Address, 0, 3),
		chainID:          big.NewInt(2153),
		evmWSAddress:     "ws://prod-testnet-us-west-2-sentry-003-public.prod.findora.org:8546",
		evmPRCAddress:    "http://prod-testnet-us-west-2-full-003-open.prod.findora.org:8545",
		blockTime:        18 * time.Second,
		fixedGiveawayWei: big.NewInt(3000000000000000), // 0.003
		maxCapWei:        big.NewInt(6000000000000000), // 0.006
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
	tempF, err := ioutil.TempFile("", "giveaway_e2e_current_gived_wei_file_*")
	s.Require().NoErrorf(err, "ioutil.TempFile:%v", err)

	srv, err := giveaway.New(
		client.New(&config.Server{
			ServerDialTimeoutSec: 9,
			ServerWSAddresses:    []string{s.evmWSAddress},
			ServerRPCAddresses:   []string{s.evmPRCAddress},
		}),
		&config.GiveawayService{
			PrivateKey:             strings.TrimPrefix(hexutil.Encode(crypto.FromECDSA(s.privateKey)), "0x"),
			HandlerTotalTimeoutSec: 30,
			SubscripTimeoutSec:     3,
			EventLogPoolSize:       9,
			FixedGiveawayWei:       s.fixedGiveawayWei,
			MaxCapWei:              s.maxCapWei,
			TokenAddresses:         []string{s.tokenAddr.String()},
			CurrentGaveWeiFilepath: tempF.Name(),
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

	privateKey, err := crypto.HexToECDSA(os.Getenv("E2E_WPK"))
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

	s.T().Logf("giveawayTestSuite, contract address: %v, tx_hash: %v", addr, tx.Hash())
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
	ctx, cancel := context.WithTimeout(context.Background(), s.blockTime*time.Duration(len(s.toAddrs))*6)
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
	for loop := 0; loop < 2; loop++ {
		for i := 0; i < len(s.toAddrs); i++ {
			toAddr := s.toAddrs[i]

			// add a simple retry here
			tx, err := s.instance.Mint(s.genAuth(ctx, c), toAddr, big.NewInt(90000000000000000))
			if err != nil {
				for i := 0; i < 3; i++ {
					tx, err = s.instance.Mint(s.genAuth(ctx, c), toAddr, big.NewInt(90000000000000000))
					if err == nil {
						break
					}
				}
			}
			s.Require().NoErrorf(err, "instance.Mint:%v", err)

			s.T().Logf(`
				    Test_E2E_Giveaway
				    [%d]mint(%d) 
				    toAddr:%v 
				    tx_hash:%v
				    `, loop, i, toAddr, tx.Hash())

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
				"[%d](%d) toAddr:%v, want:%v, got:%v",
				loop, i, toAddr, wants[i].wantBalance.Uint64(), gotBalance.Uint64(),
			)
		}
	}
}
