package gasfee

import (
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"math/big"
	"net/http"
	"os"
	"strconv"
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
	client                 client.Client
	stdoutlogger           *log.Logger
	stderrlogger           *log.Logger
	privateKey             *ecdsa.PrivateKey
	fromAddress            common.Address
	done                   chan struct{}
	crawlerTick            *time.Ticker
	refundTick             *refundTicker
	filterQuery            ethereum.FilterQuery
	refunderTimeout        time.Duration
	crawlerTimeout         time.Duration
	curBlockNumberFilepath string
	refundThreshold        *big.Float
	refundMaxCapWei        *big.Int
	refundedWeiFilepath    string
	refundedListFilepath   string
	prices                 *prices
	crawlingAddr           string
	numerator              common.Address
	denominator            common.Address
	mapper                 map[common.Address]*crawlingMate
	blockInterval          int
	baseRate               *big.Float
	dynGasPriceMax         *big.Float
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
	var denominator, numerator common.Address

	for cp, mate := range conf.CrawlingMapper {
		tokenAddr := common.HexToAddress(mate.TokenAddress)
		currencyPair := config.CurrencyPair(strings.ToUpper(strings.TrimSpace(string(cp))))

		mapper[tokenAddr] = &crawlingMate{
			priceKind:    mate.PriceKind,
			currencyPair: currencyPair,
			decimal:      mate.Decimal,
		}

		switch currencyPair {
		case conf.Denominator:
			denominator = tokenAddr
			addresses = append(addresses, tokenAddr)
		case conf.Numerator:
			numerator = tokenAddr
			addresses = append(addresses, tokenAddr)
		default:
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
		refundTick:             &refundTicker{period: 24 * time.Hour, at: conf.RefundEveryDayAt},
		refunderTimeout:        time.Duration(conf.RefunderTotalTimeoutSec) * time.Second,
		crawlerTimeout:         time.Duration(conf.CrawlerTotalTimeoutSec) * time.Second,
		refundThreshold:        conf.RefundThreshold,
		prices:                 &prices{mux: new(sync.RWMutex), values: make(map[common.Address]*big.Float)},
		crawlingAddr:           conf.CrawlingAddress,
		denominator:            denominator,
		numerator:              numerator,
		mapper:                 mapper,
		curBlockNumberFilepath: conf.CurrentBlockNumberFilepath,
		blockInterval:          conf.RefunderScrapBlockStep,
		refundMaxCapWei:        conf.RefundMaxCapWei,
		refundedWeiFilepath:    conf.RefundedWeiFilepath,
		refundedListFilepath:   conf.RefundedListFilepath,
		baseRate:               conf.RefundBaseRateWei,
		dynGasPriceMax:         conf.RefundDynamicGasPriceLimit,
	}

	s.resetPrices()
	s.Start()

	curBlockNumB, err := ioutil.ReadFile(s.curBlockNumberFilepath)
	if err != nil {
		return nil, fmt.Errorf("refunder read file:%q failed:%w", s.curBlockNumberFilepath, err)
	}
	curBlockNum, _ := binary.Uvarint(curBlockNumB)
	if conf.RefunderStartBlockNumber > curBlockNum {
		curBlockNumB = make([]byte, binary.MaxVarintLen64)
		binary.PutUvarint(curBlockNumB, conf.RefunderStartBlockNumber)
		if err := ioutil.WriteFile(s.curBlockNumberFilepath, curBlockNumB, os.ModeType); err != nil {
			return nil, fmt.Errorf("refunder write file:%q failed:%w", s.curBlockNumberFilepath, err)
		}
	}

	s.stdoutlogger.Printf("gasfeeService starting: %+v\n", conf)

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

func (p *prices) set(k common.Address, v float64) {
	p.mux.Lock()
	defer p.mux.Unlock()
	p.values[k] = big.NewFloat(v)
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
				s.stdoutlogger.Println("crawler ticked")
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
				s.stdoutlogger.Println("refunder ticked")
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
	if s.crawlerTick != nil {
		s.crawlerTick.Stop()
	}

	if s.refundTick != nil {
		s.refundTick.timer.Stop()
	}

	close(s.done)
}

var (
	ErrNotOverThreshold = errors.New("transaction value is not over the threshold")
	ErrAlreadyRefunded  = errors.New("address has been refunded already")
)

func (s *Service) refunder() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.refunderTimeout)
	defer cancel()

	c, err := s.client.DialRPC()
	if err != nil {
		return fmt.Errorf("refunder client.DialRPC failed:%w", err)
	}

	refundedListB, err := ioutil.ReadFile(s.refundedListFilepath)
	if err != nil {
		return fmt.Errorf("refunder read file:%q failed:%w", s.refundedListFilepath, err)
	}

	var refundedList []string
	if err := json.Unmarshal(refundedListB, &refundedList); err != nil {
		return fmt.Errorf("refunder json unmarshal refunded list failed:%w", err)
	}

	refundedMap := map[string]struct{}{}
	for _, addr := range refundedList {
		refundedMap[addr] = struct{}{}
	}

	handing := func(log *types.Log, dynGasPrice *big.Float) error {
		if len(log.Topics) != 3 {
			return fmt.Errorf("refunder receive not expecting format on topics:%v, tx_hash:%s", log.Topics, log.TxHash)
		}

		value := big.NewFloat(0.0).SetInt(common.BytesToHash(log.Data).Big())
		toAddr := common.BytesToAddress(common.TrimLeftZeroes(log.Topics[2].Bytes()))
		if _, exists := refundedMap[toAddr.String()]; exists {
			s.stdoutlogger.Printf("to_address:%s already refunded", toAddr)
			return ErrAlreadyRefunded
		}

		mate, ok := s.mapper[log.Address]
		if !ok {
			return fmt.Errorf("refunder cannot find decimal from token_address:%s, tx_hash:%s", log.Address, log.TxHash)
		}

		refundedWeiB, err := ioutil.ReadFile(s.refundedWeiFilepath)
		if err != nil {
			return fmt.Errorf("refunder read file:%q failed:%w", s.refundedWeiFilepath, err)
		}
		refundedWei := big.NewInt(0).SetBytes(refundedWeiB)

		denominator := s.prices.get(s.denominator)
		numerator := s.prices.get(s.numerator)
		toPrice := s.prices.get(log.Address)

		transferedToken := value.Quo(value, big.NewFloat(math.Pow10(mate.decimal)))
		transferedPrice := transferedToken.Mul(transferedToken, toPrice)

		s.stdoutlogger.Printf(`refunder handling, to_address:%s, value:%v, threshold:%v, tx_hash:%s, token_address:%s, decimal:%d, (numerator:%v / denominator:%v), target_price:%v, refunded_wei:%s, refund_max_cap_wei:%s, dynamic_gas_price:%v`,
			toAddr, value, s.refundThreshold, log.TxHash, log.Address, mate.decimal, numerator, denominator, toPrice, refundedWei, s.refundMaxCapWei, dynGasPrice,
		)

		if transferedPrice.Cmp(s.refundThreshold) <= 0 || refundedWei.Cmp(s.refundMaxCapWei) >= 0 {
			s.stdoutlogger.Printf("to_address:%s not passing the threshold", toAddr)
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

		fluctuation := big.NewFloat(0).Quo(numerator, denominator)
		var baseRate *big.Float
		if dynGasPrice != nil {
			baseRate = big.NewFloat(0).Mul(dynGasPrice, s.baseRate)
		} else {
			baseRate = big.NewFloat(0).Add(big.NewFloat(0), s.baseRate)
		}

		refundValue, _ := big.NewFloat(0).Mul(baseRate, fluctuation).Int(nil)
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

		refundedWei = refundedWei.Add(refundedWei, refundValue)
		if err := ioutil.WriteFile(s.refundedWeiFilepath, refundedWei.Bytes(), os.ModeType); err != nil {
			return fmt.Errorf("refunder write file:%q failed:%w", s.refundedWeiFilepath, err)
		}

		s.stdoutlogger.Printf(`refunder success, to_address:%s, tx_hash:%s, token_address:%s, refund_tx_hash:%s, refund_value:%s, refunded_wei:%s`,
			toAddr, log.TxHash, log.Address, tx.Hash(), refundValue, refundedWei,
		)

		refundedList = append(refundedList, toAddr.String())
		refundedMap[toAddr.String()] = struct{}{}
		return nil
	}

	latestBlockNumber, err := c.BlockNumber(ctx)
	if err != nil {
		return fmt.Errorf("refunder c.BlockNumber failed:%w", err)
	}

	curBlockNumB, err := ioutil.ReadFile(s.curBlockNumberFilepath)
	if err != nil {
		return fmt.Errorf("refunder read file:%q failed:%w", s.curBlockNumberFilepath, err)
	}
	curBlockNum, _ := binary.Uvarint(curBlockNumB)

	blockNumberDiff := latestBlockNumber - curBlockNum
	curBlockNumber := curBlockNum
	s.stdoutlogger.Printf("blockFrom:%v, blockNumberDiff:%v", curBlockNum, blockNumberDiff)

	var dynGasprice *big.Float
	if s.dynGasPriceMax != nil {
		p, err := s.client.DynamicGasPrice(ctx)
		if err != nil {
			return fmt.Errorf("refunder get DynamicGasPrice failed:%w", err)
		}
		dynGasprice = big.NewFloat(0).SetInt(p)
		if dynGasprice.Cmp(s.dynGasPriceMax) > 1 {
			dynGasprice = big.NewFloat(0).Set(s.dynGasPriceMax)
		}
	}

	var errs []string
	for n := 0; n < int(blockNumberDiff); n += s.blockInterval {
		s.filterQuery.FromBlock = big.NewInt(0).SetUint64(curBlockNumber)
		curBlockNumber += uint64(s.blockInterval)
		if curBlockNumber > latestBlockNumber {
			curBlockNumber -= curBlockNumber - latestBlockNumber
		}
		s.filterQuery.ToBlock = big.NewInt(0).SetUint64(curBlockNumber)
		// avoiding the next fromBlock repeat with the current toBlock
		curBlockNumber++
		n++

		logs, err := c.FilterLogs(ctx, s.filterQuery)
		if err != nil {
			errs = append(errs, fmt.Sprintf("refunder c.FilterLogs failed:%v", err))
			continue
		}

		for _, log := range logs {
			if err := handing(&log, dynGasprice); err != nil {
				switch err {
				case ErrAlreadyRefunded, ErrNotOverThreshold:
					// skip those two cases
				default:
					errs = append(errs, err.Error())
				}
			}
		}
	}

	curBlockNum += blockNumberDiff
	curBlockNumB = make([]byte, binary.MaxVarintLen64)
	binary.PutUvarint(curBlockNumB, curBlockNum)
	if err := ioutil.WriteFile(s.curBlockNumberFilepath, curBlockNumB, os.ModeType); err != nil {
		return fmt.Errorf("refunder write file:%q failed:%w", s.curBlockNumberFilepath, err)
	}

	refundedListB, err = json.Marshal(refundedList)
	if err != nil {
		return fmt.Errorf("refunder json marshal refunded list failed:%w", err)
	}

	if err := ioutil.WriteFile(s.refundedListFilepath, refundedListB, os.ModeType); err != nil {
		return fmt.Errorf("refunder write file:%q failed:%w", s.refundedListFilepath, err)
	}

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
		// adding exceptions for pairs
		// 1. USDT_USDT
		// 2. USDC_USDT
		// 3. BUSD_USDT
		// they are all 1:1 so no need to crawle
		switch mate.currencyPair {
		case config.CurrencyPair("USDT_USDT"), config.CurrencyPair("USDC_USDT"), config.CurrencyPair("BUSD_USDT"):
			s.prices.set(tokenAddr, 1.0)
			return nil
		}

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
		data := make([][]string, 0, 1)
		if err := json.NewDecoder(rep.Body).Decode(&data); err != nil {
			return fmt.Errorf("crawler json decode failed:%w, currency_pair:%s, token_address:%s", err, mate.currencyPair, tokenAddr)
		}

		if len(data) == 0 || len(data[0]) < 5 {
			return fmt.Errorf("crawler http response not correct:%v, currency_pair:%s, token_address:%s", err, mate.currencyPair, tokenAddr)
		}

		high, err := strconv.ParseFloat(data[0][3], 64)
		if err != nil {
			return fmt.Errorf("crawler parse highest price failed:%w, currency_pair:%s, token_address:%s", err, mate.currencyPair, tokenAddr)
		}

		low, err := strconv.ParseFloat(data[0][4], 64)
		if err != nil {
			return fmt.Errorf("crawler parse lowest price failed:%w, currency_pair:%s, token_address:%s", err, mate.currencyPair, tokenAddr)
		}

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
