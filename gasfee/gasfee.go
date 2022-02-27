package gasfee

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"math/big"
	"net/http"
	"os"
	"strings"
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
	refundThreshold *big.Float
	refundMaxCapWei *big.Int
	refundedWei     *big.Int
	prices          *prices
	crawlingAddr    string
	fraTokenAddr    common.Address
	mapper          map[common.Address]*crawlingMate
	blockInterval   int
}

type crawlingMate struct {
	priceKind    config.PriceKind
	currencyPair config.CurrencyPair
	decimal      int
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

	mapper := make(map[common.Address]*crawlingMate)
	addresses := make([]common.Address, 0, len(conf.CrawlingMapper))
	var fraTokenAddr common.Address

	for cp, mate := range conf.CrawlingMapper {
		tokenAddr := common.HexToAddress(mate.TokenAddress)
		currencyPair := config.CurrencyPair(strings.ToUpper(strings.TrimSpace(string(cp))))

		mapper[tokenAddr] = &crawlingMate{
			priceKind:    mate.PriceKind,
			currencyPair: currencyPair,
			decimal:      mate.Decimal,
		}

		if currencyPair == config.CurrencyPair("FRA_USDT") {
			fraTokenAddr = tokenAddr
		} else {
			addresses = append(addresses, tokenAddr)
		}
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
		prices:          &prices{mux: new(sync.RWMutex)},
		crawlingAddr:    conf.CrawlingAddress,
		fraTokenAddr:    fraTokenAddr,
		mapper:          mapper,
		curBlockNumber:  conf.RefunderStartBlockNumber,
		blockInterval:   conf.RefunderScrapBlockStep,
		refundMaxCapWei: conf.RefundMaxCapWei,
		refundedWei:     big.NewInt(0),
	}

	s.resetPrices()
	s.Start()
	return s, nil
}

func (s *Service) resetPrices() {
	s.prices.mux.Lock()
	defer s.prices.mux.Unlock()

	for tokenAddr, mate := range s.mapper {
		switch mate.priceKind {
		case config.Highest:
			s.prices.values[tokenAddr] = big.NewFloat(math.SmallestNonzeroFloat64)
		case config.Lowest:
			s.prices.values[tokenAddr] = big.NewFloat(math.MaxFloat64)
		}
	}
}

type prices struct {
	mux    *sync.RWMutex
	values map[common.Address]*big.Float
}

func (p *prices) get(k common.Address) *big.Float {
	p.mux.RLock()
	defer p.mux.RUnlock()
	return p.values[k]
}

func (p *prices) cmpThenSet(cmpk common.Address, high, low float64, cond config.PriceKind) {
	p.mux.Lock()
	defer p.mux.Unlock()

	curv := p.values[cmpk]
	switch cond {
	case config.Highest:
		newv := big.NewFloat(high)
		if newv.Cmp(curv) > 0 {
			p.values[cmpk] = newv
		}
	case config.Lowest:
		newv := big.NewFloat(low)
		if newv.Cmp(curv) < 0 {
			p.values[cmpk] = newv
		}
	}
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
				s.resetPrices()
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

var ErrNotOverThreshold = errors.New("transaction value is not over the threshold")

func (s *Service) refunder() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.refunderTimeout)
	defer cancel()

	c, err := s.client.DialRPC()
	if err != nil {
		return fmt.Errorf("refunder client.DialRPC failed:%w", err)
	}

	handing := func(log *types.Log) error {
		if len(log.Topics) != 3 {
			return fmt.Errorf("refunder receive not expecting format on topics:%v, tx_hash:%s", log.Topics, log.TxHash)
		}

		value := big.NewFloat(0.0).SetInt(common.BytesToHash(log.Data).Big())
		toAddr := common.BytesToAddress(common.TrimLeftZeroes(log.Topics[2].Bytes()))
		mate, ok := s.mapper[log.Address]
		if !ok {
			return fmt.Errorf("refunder cannot find decimal from token_address:%s, tx_hash:%s", log.Address, log.TxHash)
		}

		fraPrice := s.prices.get(s.fraTokenAddr)
		toPrice := s.prices.get(log.Address)

		transferedToken := value.Quo(value, big.NewFloat(math.Pow10(mate.decimal)))
		transferedPrice := transferedToken.Mul(transferedToken, toPrice)

		s.stdoutlogger.Printf(`
refunder handling:
to_address:		%s
value:			%s
threshold:		%s
tx_hash:		%s
token_address:		%s
decimal:		%d
fra_price:		%s
target_price:		%s
transfered_token:	%s
transfered_price:	%s
refunded_wei:		%s
refund_max_cap_wei:	%s
`, toAddr, value, s.refundThreshold, log.TxHash, log.Address, mate.decimal, fraPrice, toPrice, transferedToken, transferedPrice, s.refundedWei, s.refundMaxCapWei)

		if transferedPrice.Cmp(s.refundThreshold) <= 0 || s.refundedWei.Cmp(s.refundMaxCapWei) >= 0 {
			return ErrNotOverThreshold
		}

		nonce, err := c.PendingNonceAt(ctx, s.fromAddress)
		if err != nil {
			return fmt.Errorf("refunder PendingNonceAt failed:%w, tx_hash:%s, addr:%s", err, log.TxHash, log.Address)
		}

		gasPrice, err := c.SuggestGasPrice(ctx)
		if err != nil {
			return fmt.Errorf("refunder SuggestGasPrice failed:%w, tx_hash:%s, addr:%s", err, log.TxHash, log.Address)
		}

		chainID, err := c.NetworkID(ctx)
		if err != nil {
			return fmt.Errorf("refunder NetworkID failed:%w, tx_hash:%s, addr:%s", err, log.TxHash, log.Address)
		}

		base := big.NewFloat((0.00053251 * 0.5) * 1000000000000000000)
		fluctuation := fraPrice.Quo(fraPrice, toPrice)
		refundValue, _ := base.Mul(base, fluctuation).Int(nil)
		tx, err := types.SignTx(
			types.NewTx(&types.LegacyTx{
				Nonce: nonce,
				// recipient address
				To: &toAddr,
				// wei(10^18)
				Value: refundValue,
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

		s.refundedWei = s.refundedWei.Add(s.refundedWei, refundValue)

		s.stdoutlogger.Printf(`
refunder success:
to_address:		%s
tx_hash:		%s
token_address:		%s
refund_tx_hash:		%s
refund_value:		%s
`, toAddr, log.TxHash, log.Address, tx.Hash(), refundValue)

		return nil
	}

	latestBlockNumber, err := c.BlockNumber(ctx)
	if err != nil {
		return fmt.Errorf("refunder c.BlockNumber failed:%w", err)
	}

	blockNumberDiff := latestBlockNumber - s.curBlockNumber
	curBlockNumber := s.curBlockNumber
	var errs []string

	for n := 0; n < int(blockNumberDiff); n += s.blockInterval {
		s.filterQuery.FromBlock = big.NewInt(0).SetUint64(curBlockNumber)
		curBlockNumber += uint64(s.blockInterval)
		if curBlockNumber > latestBlockNumber {
			curBlockNumber -= curBlockNumber - latestBlockNumber
		}
		s.filterQuery.ToBlock = big.NewInt(0).SetUint64(curBlockNumber)
		// avoiding the next fromBlock repeat with the current toBlock
		curBlockNumber += 1
		n += 1

		logs, err := c.FilterLogs(ctx, s.filterQuery)
		if err != nil {
			errs = append(errs, fmt.Sprintf("refunder c.FilterLogs failed:%v", err))
			continue
		}

		for _, log := range logs {
			if err := handing(&log); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}

	s.curBlockNumber += blockNumberDiff

	if errs != nil {
		return fmt.Errorf(strings.Join(errs, "\n"))
	}
	return nil
}

// https://www.gate.io/docs/apiv4/en/#market-candlesticks
func (s *Service) crawler() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.crawlerTimeout)
	defer cancel()

	handling := func(tokenAddr common.Address, mate *crawlingMate) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.crawlingAddr, nil)
		if err != nil {
			return fmt.Errorf("crawler http.NewRequestWithContext failed:%w", err)
		}

		q := req.URL.Query()
		q.Add("currency_pair", string(mate.currencyPair))
		q.Add("interval", "15m")
		q.Add("limit", "1")

		req.Header.Add("Accept", "application/json")
		req.URL.RawQuery = q.Encode()

		rep, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("crawler http.DefaultClient.Do failed:%w, currency_pair:%s, token_address:%s", err, mate.currencyPair, tokenAddr)
		}
		defer rep.Body.Close()

		// curl -H 'Accept: application/json' -X GET https://api.gateio.ws/api/v4/spot/candlesticks\?currency_pair\=FRA_USDT\&interval\=15m\&limit\=1
		// [[unix_timestamp, trading_volume, close_price, highest_price, lowest_price, open_price]]
		// [["1645749900","2839.79160470986265","0.01815","0.01897","0.01793","0.01889"]]
		data := make([][]float64, 0, 1)
		data = append(data, make([]float64, 0, 6))
		if err := json.NewDecoder(rep.Body).Decode(&data); err != nil {
			return fmt.Errorf("crawler FRA json decode failed:%w, currency_pair:%s, token_address:%s", err, mate.currencyPair, tokenAddr)
		}

		if len(data) == 0 || len(data[0]) != 6 {
			return fmt.Errorf("crawler http response not correct:%v, currency_pair:%s, token_address:%s", err, mate.currencyPair, tokenAddr)
		}

		high := data[0][3]
		low := data[0][4]
		s.prices.cmpThenSet(tokenAddr, high, low, mate.priceKind)

		return nil
	}

	var errs []string
	for tokenAddr, mate := range s.mapper {
		if err := handling(tokenAddr, mate); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if errs != nil {
		return fmt.Errorf(strings.Join(errs, "\n"))
	}
	return nil
}
