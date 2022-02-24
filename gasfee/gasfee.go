package gasfee

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"sync"
	"time"

	"github.com/FindoraNetwork/refunder/client"
	"github.com/FindoraNetwork/refunder/config"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type Service struct {
	client          client.Client
	stdoutlogger    *log.Logger
	stderrlogger    *log.Logger
	privateKey      *ecdsa.PrivateKey
	fromAddress     common.Address
	done            chan struct{}
	crawlerTick     *time.Ticker
	refundTick      *refundTicker
	filterQuery     ethereum.FilterQuery
	refunderTimeout time.Duration
	crawlerTimeout  time.Duration
	curBlockNumber  uint64
	refundThreshold *big.Int
	prices          *sync.Map
}

func New(c client.Client, conf *config.GasfeeService) (*Service, error) {
	privateKey, err := crypto.HexToECDSA(conf.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("new on crypto.HexToECDSA private key failed:%w", err)
	}

	publicKey, ok := privateKey.Public().(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("new on casting public key to ECDSA failed")
	}

	addresses := make([]common.Address, 0, len(conf.TokenAddresses))
	for _, address := range conf.TokenAddresses {
		addresses = append(addresses, common.HexToAddress(address))
	}

	s := &Service{
		client:       c,
		privateKey:   privateKey,
		fromAddress:  crypto.PubkeyToAddress(*publicKey),
		stdoutlogger: log.New(os.Stdout, "gasfeeService:", log.Lmsgprefix),
		stderrlogger: log.New(os.Stderr, "gasfeeService:", log.Lmsgprefix),
		done:         make(chan struct{}),
		crawlerTick:  time.NewTicker(time.Duration(conf.CrawleInEveryMinutes) * time.Minute),
		filterQuery: ethereum.FilterQuery{
			Addresses: addresses,
			Topics: [][]common.Hash{
				{crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))},
				{common.BytesToHash([]byte(""))},
			},
		},
		refundTick:      &refundTicker{period: 24 * time.Hour, at: conf.RefundEveryDayAt},
		refunderTimeout: time.Duration(conf.RefunderTotalTimeoutSec) * time.Second,
		crawlerTimeout:  time.Duration(conf.CrawlerTotalTimeoutSec) * time.Second,
		refundThreshold: conf.RefundThreshold,
		prices:          &sync.Map{},
	}

	s.Start()
	return s, nil
}

type refundTicker struct {
	timer  *time.Timer
	period time.Duration
	at     time.Time
}

// adjusting the refund ticker must to be tick at RefundEveryDayAt GMT time
func (r *refundTicker) updateTimer() {
	hh := r.at.Hour()
	mm := r.at.Minute()
	ss := r.at.Second()
	now := time.Now().UTC()

	nextTick := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, ss, 0, time.UTC)
	if !nextTick.After(now) {
		nextTick = nextTick.Add(r.period)
	}

	diff := nextTick.Sub(now)
	if r.timer == nil {
		r.timer = time.NewTimer(diff)
	} else {
		r.timer.Reset(diff)
	}
}

// Start spawns two goroutines:
// 1. a crawler ---> crawling gate.io to get the usdt price
// 2. a refund  ---> do the refunding action to recipients
func (s *Service) Start() {
	s.refundTick.updateTimer()

	go func() {
		for {
			select {
			case <-s.done:
				return
			case <-s.crawlerTick.C:
				if err := s.crawler(); err != nil {
					s.stderrlogger.Println(err)
				}
			}
		}
	}()

	go func() {
		for {
			select {
			case <-s.done:
				return
			case <-s.refundTick.timer.C:
				if err := s.refunder(); err != nil {
					s.stderrlogger.Println(err)
				}
				s.refundTick.updateTimer()
			}
		}
	}()
}

// Close stops the fork out goroutines from Start method
func (s *Service) Close() {
	s.crawlerTick.Stop()
	s.refundTick.timer.Stop()
	close(s.done)
}

var (
	ErrTxIsPending      = errors.New("transaction is pending")
	ErrNotOverThreshold = errors.New("transaction value is not over the threshold")
)

func (s *Service) refunder() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.refunderTimeout)
	defer cancel()

	c, err := s.client.DialRPC()
	if err != nil {
		return fmt.Errorf("refunder client.DialRPC failed:%w", err)
	}

	latestBlockNumber, err := c.BlockNumber(ctx)
	if err != nil {
		return fmt.Errorf("refunder c.BlockNumber failed:%w", err)
	}

	s.filterQuery.FromBlock = big.NewInt(0).SetUint64(s.curBlockNumber)
	s.filterQuery.ToBlock = big.NewInt(0).SetUint64(latestBlockNumber)

	logs, err := c.FilterLogs(ctx, s.filterQuery)
	if err != nil {
		return fmt.Errorf("refunder c.FilterLogs failed:%w", err)
	}

	handing := func(log *types.Log) error {
		tx, isPending, err := c.TransactionByHash(ctx, log.TxHash)
		if err != nil {
			return fmt.Errorf("refunder c.TransactionByHash failed:%w, tx_hash:%s, addr:%s", err, log.TxHash, log.Address)
		}

		s.stdoutlogger.Printf(`
refunder handling:
to_address:	%s
tx_hash:	%s
value:		%s
threshold:	%s
`, log.Address, tx.Hash(), tx.Value(), s.refundThreshold)

		if isPending {
			return ErrTxIsPending
		}

		if tx.Value().Cmp(s.refundThreshold) < 0 {
			return ErrNotOverThreshold
		}

		nonce, err := c.PendingNonceAt(ctx, s.fromAddress)
		if err != nil {
			return fmt.Errorf("refunder PendingNonceAt failed:%w, tx_hash:%s, addr:%s", err, tx.Hash(), log.Address)
		}

		gasPrice, err := c.SuggestGasPrice(ctx)
		if err != nil {
			return fmt.Errorf("refunder SuggestGasPrice failed:%w, tx_hash:%s, addr:%s", err, tx.Hash(), log.Address)
		}

		chainID, err := c.NetworkID(ctx)
		if err != nil {
			return fmt.Errorf("refunder NetworkID failed:%w, tx_hash:%s, addr:%s", err, tx.Hash(), log.Address)
		}

		fraPrice, ok := s.prices.Load(chainID)
		if !ok {
			return fmt.Errorf("refunder get fra price not ok:%s, tx_hash:%s, addr:%s", chainID, tx.Hash(), log.Address)
		}
		fraUSDT, ok := fraPrice.(*big.Int)
		if !ok {
			return fmt.Errorf("refunder cast fra usdt not ok:%s, tx_hash:%s, addr:%s", chainID, tx.Hash(), log.Address)
		}

		destPrice, ok := s.prices.Load(tx.ChainId())
		if !ok {
			return fmt.Errorf("refunder get dest price not ok:%s, tx_hash:%s, addr:%s", chainID, tx.Hash(), log.Address)
		}
		destUSDT, ok := destPrice.(*big.Int)
		if !ok {
			return fmt.Errorf("refunder cast dest usdt not ok:%s, tx_hash:%s, addr:%s", chainID, tx.Hash(), log.Address)
		}

		base := big.NewInt((0.00053251 * 0.5) * 1000000000000000000)
		fluctuation := big.NewInt(0).Div(fraUSDT, destUSDT)
		value := base.Mul(base, fluctuation)
		tx, err = types.SignTx(
			types.NewTx(&types.LegacyTx{
				Nonce: nonce,
				// recipient address
				To: &log.Address,
				// wei(10^18)
				Value: value,
				// 21000 gas is the default value for transfering native token
				Gas:      uint64(21000),
				GasPrice: gasPrice,
				// 0x data is the default value for transfering native token
				Data: nil,
			}),
			types.NewEIP155Signer(chainID),
			s.privateKey,
		)
		if err != nil {
			return fmt.Errorf("refunder SignTx failed:%w, tx_hash:%s, addr:%s", err, tx.Hash(), log.Address)
		}

		if err := c.SendTransaction(ctx, tx); err != nil {
			return fmt.Errorf("refunder SendTransaction failed:%w, tx_hash:%s, addr:%s", err, tx.Hash(), log.Address)
		}

		return nil
	}

	for _, log := range logs {
		if err := handing(&log); err != nil {
			switch err {
			default:
				s.stderrlogger.Println(err)
			}
		}
	}

	return nil
}

func (s *Service) crawler() error {
	return nil
}
