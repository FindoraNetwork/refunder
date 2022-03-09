package giveaway

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/FindoraNetwork/refunder/client"
	"github.com/FindoraNetwork/refunder/config"
	"github.com/gorilla/websocket"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type Service struct {
	client           client.Client
	eventLogPoolSize int
	done             chan struct{}

	stdoutlogger *log.Logger
	stderrlogger *log.Logger

	subscribeTimeout    time.Duration
	handlerTotalTimeout time.Duration

	filterQuery ethereum.FilterQuery

	privateKey         *ecdsa.PrivateKey
	fromAddress        common.Address
	maxCapWei          *big.Int
	fixedGiveawayWei   *big.Int
	curGaveWeiFilepath string
}

func New(c client.Client, conf *config.GiveawayService) (*Service, error) {
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
		client:              c,
		stdoutlogger:        log.New(os.Stdout, "giveawayService:", log.Lmsgprefix),
		stderrlogger:        log.New(os.Stderr, "giveawayService:", log.Lmsgprefix),
		done:                make(chan struct{}),
		subscribeTimeout:    time.Duration(conf.SubscripTimeoutSec) * time.Second,
		handlerTotalTimeout: time.Duration(conf.HandlerTotalTimeoutSec) * time.Second,
		eventLogPoolSize:    conf.EventLogPoolSize,
		filterQuery: ethereum.FilterQuery{
			Addresses: addresses,
			Topics: [][]common.Hash{
				{crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))},
				{common.BytesToHash([]byte(""))},
			},
		},
		privateKey:         privateKey,
		fromAddress:        crypto.PubkeyToAddress(*publicKey),
		fixedGiveawayWei:   conf.FixedGiveawayWei,
		maxCapWei:          conf.MaxCapWei,
		curGaveWeiFilepath: conf.CurrentGaveWeiFilepath,
	}

	if err := s.Start(); err != nil {
		return nil, fmt.Errorf("new on starting service failed:%w", err)
	}

	s.stdoutlogger.Println("giveawayService starting:", conf)

	return s, nil
}

// Start fork out a goroutine to listen to specific event log which is defined in filterQuery field then bypass into the handler
func (s *Service) Start() error {
	subscribing := func() (ethereum.Subscription, chan types.Log, error) {
		c, err := s.client.DialWS()
		if err != nil {
			return nil, nil, fmt.Errorf("start dialing to server failed:%w", err)
		}

		logChan := make(chan types.Log, s.eventLogPoolSize)
		ctx, cancel := context.WithTimeout(context.Background(), s.subscribeTimeout)
		defer cancel()

		sub, err := c.SubscribeFilterLogs(ctx, s.filterQuery, logChan)
		if err != nil {
			return nil, nil, fmt.Errorf("subscribe filter logs failed:%w", err)
		}
		return sub, logChan, nil
	}

	sub, logChan, suberr := subscribing()
	if suberr != nil {
		return suberr
	}

	go func() {
		for {
			select {
			case <-s.done:
				sub.Unsubscribe()
				return
			case err := <-sub.Err():
				switch {
				case websocket.IsCloseError(err, websocket.CloseAbnormalClosure):
					s.stdoutlogger.Println("websocket.CloseAbnormalClosure try to reconnect")
					sub, logChan, suberr = subscribing()
					if suberr != nil {
						s.stderrlogger.Printf("websocket.CloseAbnormalClosure reconnect failed:%v, service stop", suberr)
						return
					}
				case os.IsTimeout(err):
					s.stdoutlogger.Println("websocket.read i/o timeout try to reconnect")
					sub, logChan, suberr = subscribing()
					if suberr != nil {
						s.stderrlogger.Printf("websocket.read i/o timeout reconnect failed:%v, service stop", suberr)
						return
					}
				case err == nil:
					// this is weird, but it's really happening...
					s.stdoutlogger.Println("websocket received nil error try to reconnect")
					sub, logChan, suberr = subscribing()
					if suberr != nil {
						s.stderrlogger.Printf("websocket received nil error reconnect failed:%v, service stop", suberr)
						return
					}
				default:
					s.stderrlogger.Printf("subscribe websocket receive error:%v", err)
				}

			case vlog := <-logChan:
				if err := s.handler(vlog); err != nil {
					switch err {
					case ErrNotEligible:
						continue
					default:
						s.stderrlogger.Println(err)
					}
				}

			}
		}
	}()
	return nil
}

// Close stops the fork out goroutine from Start method
func (s *Service) Close() {
	close(s.done)
}

var ErrNotEligible = errors.New("not eligible with the condition")

func (s *Service) handler(vlog types.Log) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.handlerTotalTimeout)
	defer cancel()

	// for searching logs usage to know which group of logs are in the same request
	txHash := vlog.TxHash.String()

	if len(vlog.Topics) != 3 {
		return fmt.Errorf("handler receive not expecting format on topics:%v, tx_hash:%s", vlog.Topics, txHash)
	}

	curGivedWeiB, err := ioutil.ReadFile(s.curGaveWeiFilepath)
	if err != nil {
		return fmt.Errorf("handler open file:%q failed:%w", s.curGaveWeiFilepath, err)
	}
	curGivedWei := big.NewInt(0).SetBytes(curGivedWeiB)

	toAddress := common.BytesToAddress(common.TrimLeftZeroes(vlog.Topics[2].Bytes()))
	blockNumber := big.NewInt(0).SetUint64(vlog.BlockNumber)

	c, err := s.client.DialRPC()
	if err != nil {
		return fmt.Errorf("handler client dialing failed:%w, tx_hash:%s", err, txHash)
	}
	defer c.Close()

	// there has an issue if specific the blockNumber
	// BalanceAt failed: block number: 200408 exceeds version range: 1..200407
	// so use latest blockNumber for
	// - BalanceAt and
	// - NonceAt
	toBalance, err := c.BalanceAt(ctx, toAddress, nil)
	if err != nil {
		return fmt.Errorf("handler toAddress BalanceAt failed:%w, tx_hash:%s, to_address:%s", err, txHash, toAddress)
	}

	toNonce, err := c.NonceAt(ctx, toAddress, nil)
	if err != nil {
		return fmt.Errorf("handler toAddress NonceAt failed:%w, tx_hash:%s, to_address:%s", err, txHash, toAddress)
	}

	s.stdoutlogger.Printf(`handler receiving, to_address:%s, to_balance:%s, to_nonce:%d, block_number:%s, fix_giveaway:%s, max_cap:%s, current_giveout:%s, tx_hash:%s`,
		toAddress,
		toBalance,
		toNonce,
		blockNumber,
		s.fixedGiveawayWei,
		s.maxCapWei,
		curGivedWei,
		txHash,
	)

	if toBalance.Cmp(big.NewInt(0)) != 0 || toNonce != 0 || curGivedWei.Cmp(s.maxCapWei) >= 0 {
		return ErrNotEligible
	}

	nonce, err := c.PendingNonceAt(ctx, s.fromAddress)
	if err != nil {
		return fmt.Errorf("handler PendingNonceAt failed:%w, tx_hash:%s", err, txHash)
	}

	gasPrice, err := c.SuggestGasPrice(ctx)
	if err != nil {
		return fmt.Errorf("handler SuggestGasPrice failed:%w, tx_hash:%s", err, txHash)
	}

	chainID, err := c.NetworkID(ctx)
	if err != nil {
		return fmt.Errorf("handler NetworkID failed:%w, tx_hash:%s", err, txHash)
	}

	tx, err := types.SignTx(
		types.NewTx(&types.LegacyTx{
			Nonce: nonce,
			// recipient address
			To: &toAddress,
			// wei(10^18)
			Value: s.fixedGiveawayWei,
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
		return fmt.Errorf("handler SignTx failed:%w, tx_hash:%s", err, txHash)
	}

	if err := c.SendTransaction(ctx, tx); err != nil {
		return fmt.Errorf("handler SendTransaction failed:%w, tx_hash:%s", err, txHash)
	}

	curGivedWei = curGivedWei.Add(curGivedWei, s.fixedGiveawayWei)
	if err := ioutil.WriteFile(s.curGaveWeiFilepath, curGivedWei.Bytes(), os.ModeType); err != nil {
		return fmt.Errorf("handler write file:%q failed:%w", s.curGaveWeiFilepath, err)
	}

	s.stdoutlogger.Printf(`handler success, to_address:%v, block_number:%v, current_giveout:%v, current_nonce:%v, tx_hash:%v`,
		toAddress,
		blockNumber,
		curGivedWei,
		nonce,
		txHash,
	)

	return nil
}
