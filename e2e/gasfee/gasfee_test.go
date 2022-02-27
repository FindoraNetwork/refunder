// //go:build e2e
// // +build e2e

package e2e_test

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
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

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/suite"
)

type gasfeeTestSuite struct {
	suite.Suite

	chainID    *big.Int
	instance   *contract.Contract
	privateKey *ecdsa.PrivateKey
	fromAddr   common.Address
	tokenAddr  common.Address
	toAddrs    []common.Address
	serv       *gasfee.Service
	// evmWSAddress     string
	evmPRCAddress string
	// blockTime        time.Duration
	// fixedGiveawayWei *big.Int
	// maxCapWei        *big.Int
	gateIOServer *httptest.Server
}

func TestE2EGasfeeTestSuite(t *testing.T) {
	suite.Run(t, &gasfeeTestSuite{
		toAddrs: make([]common.Address, 0, 3),
		chainID: big.NewInt(2153),
		// evmWSAddress:     "ws://prod-testnet-us-west-2-sentry-003-public.prod.findora.org:8546",
		evmPRCAddress: "http://prod-testnet-us-west-2-full-003-open.prod.findora.org:8545",
		// blockTime:        18 * time.Second,
		// fixedGiveawayWei: big.NewInt(3000000000000000), // 0.003
		// maxCapWei:        big.NewInt(6000000000000000), // 0.006
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
	// s.setupSuiteStartService()
}

func (s *gasfeeTestSuite) setupSuiteDeployContract() {
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
				json.NewEncoder(w).Encode(`[["1645989300","2.9303208","0.0198","0.0198","0.0198","0.0198"]]`)
			case "DEMO_USDT":
				json.NewEncoder(w).Encode(`[["1645989300","2.9303208","0.0198","0.0198","0.0198","0.0198"]]`)
			default:
				w.WriteHeader(http.StatusNotAcceptable)
			}
		}
	}))
}

func (s *gasfeeTestSuite) setupSuiteStartService() {
	srv, err := gasfee.New(
		client.New(&config.Server{
			ServerDialTimeoutSec: 3,
			ServerRPCAddress:     s.evmPRCAddress,
		}),
		&config.GasfeeService{
			PrivateKey:               strings.TrimPrefix(hexutil.Encode(crypto.FromECDSA(s.privateKey)), "0x"),
			CrawleInEveryMinutes:     1,
			RefundEveryDayAt:         time.Now().UTC().Add(3 * time.Minute),
			RefunderTotalTimeoutSec:  3,
			RefunderStartBlockNumber: 0,
			RefunderScrapBlockStep:   200,
			CrawlerTotalTimeoutSec:   3,
			RefundThreshold:          big.NewFloat(0),
			RefundMaxCapWei:          big.NewInt(0),
			CrawlingAddress:          s.gateIOServer.URL,
			CrawlingMapper: map[config.CurrencyPair]*config.CrawlingMate{
				config.CurrencyPair("FRA_USDT"): {
					PriceKind:    config.Lowest,
					Decimal:      6,
					TokenAddress: "0x0000000000000000000000000000000000001000",
				},
				config.CurrencyPair("DEMO_USDT"): {
					PriceKind:    config.Highest,
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
