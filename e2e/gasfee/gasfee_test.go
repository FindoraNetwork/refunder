//go:build e2e
// +build e2e

package e2e_test

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FindoraNetwork/refunder/client"
	"github.com/FindoraNetwork/refunder/config"
	"github.com/FindoraNetwork/refunder/e2e/gasfee/contract"
	"github.com/FindoraNetwork/refunder/gasfee"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/suite"
)

type gasfeeTestSuite struct {
	suite.Suite

	chainID          *big.Int
	instance         *contract.Contract
	privateKey       *ecdsa.PrivateKey
	fromAddr         common.Address
	tokenAddr        common.Address
	toAddrs          []common.Address
	serv             *gasfee.Service
	evmPRCAddress    string
	gateIOServer     *httptest.Server
	startBlockNumber uint64
}

func TestE2EGasfeeTestSuite(t *testing.T) {
	suite.Run(t, &gasfeeTestSuite{
		toAddrs:       make([]common.Address, 0, 3),
		chainID:       big.NewInt(2153),
		evmPRCAddress: "http://prod-testnet-us-west-2-full-003-open.prod.findora.org:8545",
	})
}

func (s *gasfeeTestSuite) TearDownSuite() {
	s.serv.Close()
	s.gateIOServer.Close()
}

func (s *gasfeeTestSuite) SetupSuite() {
	// Step 1. deploy a token creation contract to do the transfer action
	s.setupSuiteDeployContract()
	// Step 2. generate some destination wallets
	s.setupSuiteGenerateWallets()
	// Step 3. setup a fake gate.io server
	s.setupSuiteFakeGateIOServer()
	// // Step 4. start gasfee service
	s.setupSuiteStartService()
}

func (s *gasfeeTestSuite) setupSuiteDeployContract() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c, err := ethclient.DialContext(ctx, s.evmPRCAddress)
	s.Require().NoErrorf(err, "ethclient.DialContext:%v", err)

	privateKey, err := crypto.HexToECDSA(os.Getenv("E2E_WPK"))
	s.Require().NoErrorf(err, "crypto.HexToECDSA:%v", err)
	s.privateKey = privateKey

	publicKey, ok := privateKey.Public().(*ecdsa.PublicKey)
	s.Require().True(ok, "privateKey.Public().(*ecdsa.PublicKey)")

	blockNumber, err := c.BlockNumber(ctx)
	s.Require().NoErrorf(err, "c.BlockNumber:%v", err)
	s.startBlockNumber = blockNumber

	fromAddr := crypto.PubkeyToAddress(*publicKey)
	s.fromAddr = fromAddr

	addr, tx, instance, err := contract.DeployContract(s.genAuth(ctx, c), c)
	s.Require().NoErrorf(err, "contract.DeployContract:%v", err)
	s.instance = instance
	s.tokenAddr = addr

	s.T().Logf("giveawayTestSuite, contract address: %v, tx_hash: %v", addr, tx.Hash())
}

func (s *gasfeeTestSuite) setupSuiteGenerateWallets() {
	for i := 0; i < 3; i++ {
		privateKey, err := crypto.GenerateKey()
		s.Require().NoErrorf(err, "crypto.GenerateKey:%v", err)
		publicKey, ok := privateKey.Public().(*ecdsa.PublicKey)
		s.Require().True(ok, "privateKey.Public().(*ecdsa.PublicKey)")
		s.toAddrs = append(s.toAddrs, crypto.PubkeyToAddress(*publicKey))
	}
}

func (s *gasfeeTestSuite) setupSuiteFakeGateIOServer() {
	s.gateIOServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Header.Get("Accept") != "application/json":
			w.WriteHeader(http.StatusForbidden)
		case r.Method != http.MethodGet:
			w.WriteHeader(http.StatusMethodNotAllowed)
		case !r.URL.Query().Has("currency_pair"):
			w.WriteHeader(http.StatusBadRequest)
		default:
			switch r.URL.Query().Get("currency_pair") {
			case "FRA_USDT":
				json.NewEncoder(w).Encode([][]string{{"1645989300", "2.9303208", "0.0198", "0.0198", "0.0196", "0.0194"}})
			case "DEMO_USDT":
				json.NewEncoder(w).Encode([][]string{{"1645995600", "152072.1349971985415", "361.6938", "364.6027", "361.3522", "363.5361"}})
			default:
				w.WriteHeader(http.StatusNotAcceptable)
			}
		}
	}))
}

func (s *gasfeeTestSuite) setupSuiteStartService() {
	tmpRefunded, err := ioutil.TempFile("", "gasfee_e2e_refunded_file_*")
	s.Require().NoErrorf(err, "ioutil.TempFile:%v", err)
	tmpCurBlock, err := ioutil.TempFile("", "gasfee_e2e_current_block_file_*")
	s.Require().NoErrorf(err, "ioutil.TempFile:%v", err)

	srv, err := gasfee.New(
		client.New(&config.Server{
			ServerDialTimeoutSec: 9,
			ServerRPCAddress:     s.evmPRCAddress,
		}),
		&config.GasfeeService{
			PrivateKey:                 strings.TrimPrefix(hexutil.Encode(crypto.FromECDSA(s.privateKey)), "0x"),
			CrawleInEveryMinutes:       1,
			RefundEveryDayAt:           time.Now().UTC().Add(3 * time.Minute),
			RefunderTotalTimeoutSec:    30,
			RefunderStartBlockNumber:   s.startBlockNumber,
			RefunderScrapBlockStep:     200,
			CrawlerTotalTimeoutSec:     3,
			RefundThreshold:            big.NewFloat(999.99),    // 999.99 USDT
			RefundMaxCapWei:            big.NewInt(14589226245), // 0.000000014589226245 wei
			CrawlingAddress:            s.gateIOServer.URL,
			RefundedWeiFilepath:        tmpRefunded.Name(),
			CurrentBlockNumberFilepath: tmpCurBlock.Name(),
			CrawlingMapper: map[config.CurrencyPair]*config.CrawlingMate{
				config.CurrencyPair("FRA_USDT"): {
					PriceKind:    config.Highest,
					Decimal:      6,
					TokenAddress: "0x0000000000000000000000000000000000001000",
				},
				config.CurrencyPair("DEMO_USDT"): {
					PriceKind:    config.Lowest,
					Decimal:      6,
					TokenAddress: s.tokenAddr.String(),
				},
			},
		})
	s.Require().NoErrorf(err, "gasfee.New:%v", err)
	s.serv = srv
}

func (s *gasfeeTestSuite) genAuth(ctx context.Context, c *ethclient.Client) *bind.TransactOpts {
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

func (s *gasfeeTestSuite) Test_E2E_Gasfee() {
	// total timeout for this test case
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()

	c, err := ethclient.DialContext(ctx, s.evmPRCAddress)
	s.Require().NoErrorf(err, "ethclient.DialContext:%v", err)

	// at mock gate.io server the Highest price of FRA_USDT
	fraPrice := big.NewFloat(0.0198)
	// at mock gate.io server the Lowest price of DEMO_USDT
	demoPrice := big.NewFloat(361.3522)
	// 361.3522 / 0.0198 = 18250.1111111
	fluctuation := fraPrice.Quo(demoPrice, fraPrice)
	// wantBalance = 18250.1111111 * 266255000000000 = 4859183333890000000 wei
	wantBalance, _ := big.NewFloat(0).Mul(gasfee.BaseRate, fluctuation).Int(nil)
	// 3 tokens of DEMO * 361.3522 USDT == 1084.0566 USDT
	mint3tokens := big.NewInt(3000000)
	type want struct {
		name        string
		toAddr      common.Address
		wantBalance *big.Int
		blockNumber *big.Int
		mint        *big.Int
	}
	wants := []want{
		{
			name:        "received refund",
			wantBalance: wantBalance,
			mint:        mint3tokens,
		},
		{
			name:        "block by threshold",
			wantBalance: big.NewInt(0),
			mint:        big.NewInt(1000000), // 1 token of DEMO == 361.3522 USDT
		},
		{
			name:        "block by max cap",
			wantBalance: big.NewInt(0),
			mint:        mint3tokens,
		},
	}

	// let the crawler scrapes at least once
	time.Sleep(time.Minute)

	// mint demo token to the receipts
	for i := 0; i < len(s.toAddrs); i++ {
		toAddr := s.toAddrs[i]
		mint := wants[i].mint

		// add a simple retry here
		tx, err := s.instance.Mint(s.genAuth(ctx, c), toAddr, mint)
		if err != nil {
			for i := 0; i < 3; i++ {
				tx, err = s.instance.Mint(s.genAuth(ctx, c), toAddr, mint)
				if err == nil {
					break
				}
			}
		}
		s.Require().NoErrorf(err, "instance.Mint:%v", err)

		s.T().Logf(`
				    Test_E2E_Gasfee
				    (%d)mint
				    toAddr:%v 
				    tx_hash:%v
				    `, i, toAddr, tx.Hash())

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

		wants[i].toAddr = toAddr
		wants[i].blockNumber = receipt.BlockNumber
	}

	// wait for the refunder runs
	time.Sleep(3 * time.Minute)

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
			"(%d)name:%s toAddr:%v, want:%s, got:%s",
			i, wants[i].name, toAddr, wants[i].wantBalance, gotBalance,
		)
	}
}
