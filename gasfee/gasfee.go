package gasfee

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/FindoraNetwork/refunder/client"
	"github.com/FindoraNetwork/refunder/config"
	"github.com/FindoraNetwork/refunder/internal/workerpool"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/websocket"
)

type setting struct {
	refundAmount    *big.Int
	refundThreshold *big.Int
}

type Service struct {
	client           *client.Client
	wp               *workerpool.WorkerPool
	eventLogPoolSize int
	done             chan struct{}

	stdoutlogger *log.Logger
	stderrlogger *log.Logger

	subscribeTimeout    time.Duration
	handlerTotalTimeout time.Duration

	filterQuery ethereum.FilterQuery

	privateKey  *ecdsa.PrivateKey
	fromAddress common.Address

	refundSettings map[common.Address]*setting
}

func New(client *client.Client, conf *config.GasFeeService) (*Service, error) {
	errLogger := log.New(os.Stderr, "gasfeeService: ", log.Lmsgprefix)

	addresses := make([]common.Address, 0, len(conf.RefundAmounts))
	refundSettings := make(map[common.Address]*setting)
	power18 := big.NewInt(0).Mul(big.NewInt(10), big.NewInt(1000000000000000000))

	for addr, amount := range conf.RefundAmounts {
		address := common.HexToAddress(addr)
		addresses = append(addresses, address)
		refundAmount := big.NewInt(amount.Refund)
		refundThreshold := big.NewInt(amount.Threshold)

		refundSettings[address] = &setting{
			refundAmount:    refundAmount.Mul(refundAmount, power18),
			refundThreshold: refundThreshold.Mul(refundThreshold, big.NewInt(1000000)),
		}
	}

	privateKey, err := crypto.HexToECDSA(conf.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("new on crypto.HexToECDSA private key failed: %w", err)
	}

	publicKey, ok := privateKey.Public().(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("new on casting public key to ECDSA failed")
	}

	s := &Service{
		client:       client,
		stdoutlogger: log.New(os.Stdout, "gasfeeService: ", log.Lmsgprefix),
		stderrlogger: errLogger,
		wp: workerpool.New(
			workerpool.WithPoolSize(conf.WorkerPoolSize),
			workerpool.WithWorkerNum(conf.WorkerPoolWorkerNum),
			workerpool.WithLogger(errLogger),
		),
		done:                make(chan struct{}),
		subscribeTimeout:    time.Duration(conf.SubscripTimeoutSec) * time.Second,
		handlerTotalTimeout: time.Duration(conf.HandlerOperationsTimeoutSec) * time.Second,
		eventLogPoolSize:    conf.EventLogPoolSize,
		filterQuery: ethereum.FilterQuery{
			Addresses: addresses,
			Topics: [][]common.Hash{
				{crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))},
				{common.BytesToHash([]byte(""))},
			},
		},
		privateKey:     privateKey,
		fromAddress:    crypto.PubkeyToAddress(*publicKey),
		refundSettings: refundSettings,
	}

	if err := s.Start(); err != nil {
		return nil, fmt.Errorf("new on starting service failed: %w", err)
	}

	return s, nil
}

func (s *Service) Start() error {
	subscribing := func() (ethereum.Subscription, chan types.Log, error) {
		c, err := s.client.Dial()
		if err != nil {
			return nil, nil, fmt.Errorf("start dialing to server failed: %w", err)
		}

		logChan := make(chan types.Log, s.eventLogPoolSize)
		ctx, cancel := context.WithTimeout(context.Background(), s.subscribeTimeout)
		defer cancel()

		sub, err := c.SubscribeFilterLogs(ctx, s.filterQuery, logChan)
		if err != nil {
			return nil, nil, fmt.Errorf("subscribe filter logs failed: %w", err)
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
				s.wp.Close()
				return
			case err := <-sub.Err():
				switch {
				case websocket.IsCloseError(err, websocket.CloseAbnormalClosure):
					s.stdoutlogger.Println("websocket.CloseAbnormalClosure try to reconnect")
					sub, logChan, suberr = subscribing()
					if suberr != nil {
						s.stderrlogger.Printf("websocket.CloseAbnormalClosure reconnect failed: %v, service stop", suberr)
						return
					}
				case os.IsTimeout(err):
					s.stdoutlogger.Println("websocket.read i/o timeout try to reconnect")
					sub, logChan, suberr = subscribing()
					if suberr != nil {
						s.stderrlogger.Printf("websocket.read i/o timeout reconnect failed: %v, service stop", suberr)
						return
					}
				default:
					s.stderrlogger.Printf("subscribe websocket receive error: %v", err)
				}

			case vlog := <-logChan:
				s.wp.PutJob(func() error {
					return s.handler(vlog)
				})

			}
		}
	}()
	return nil
}

func (s *Service) Close() {
	close(s.done)
}

func (s *Service) handler(vlog types.Log) error {
	// for searching logs usage to know what group of logs are in the same request
	txHash := vlog.TxHash.String()

	if len(vlog.Topics) != 3 {
		return fmt.Errorf("handler receive not expecting format on Topics: %v, TxHash:%s", vlog.Topics, txHash)
	}

	setting, ok := s.refundSettings[vlog.Address]
	if !ok {
		return fmt.Errorf("handler refund amounts map cannot find: %v, TxHash:%s", vlog.Address, txHash)
	}

	transfer_amount := common.BytesToHash(vlog.Data).Big()
	toAddress := common.BytesToAddress(common.TrimLeftZeroes(vlog.Topics[2].Bytes()))

	s.stdoutlogger.Printf(`gas refunder receiving:
toAddress:	 %v 
transfer_amount: %v
threshold:	 %v
th_hash:	 %v
`,
		toAddress,
		transfer_amount,
		setting.refundThreshold,
		txHash,
	)

	// if threshold is bigger than transfer amount then we skip
	if setting.refundThreshold.Cmp(transfer_amount) > 1 {
		return fmt.Errorf("handler receiving transfer_amount < threshold, TxHash:%s", txHash)
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.handlerTotalTimeout)
	defer cancel()

	c, err := s.client.Dial()
	if err != nil {
		return fmt.Errorf("handler client dialing failed: %v, TxHash:%s", err, txHash)
	}
	defer c.Close()

	nonce, err := c.PendingNonceAt(ctx, s.fromAddress)
	if err != nil {
		return fmt.Errorf("handler PendingNonceAt failed: %w, TxHash:%s", err, txHash)
	}

	gasPrice, err := c.SuggestGasPrice(ctx)
	if err != nil {
		return fmt.Errorf("handler SuggestGasPrice failed: %w, TxHash:%s", err, txHash)
	}

	chainID, err := c.NetworkID(ctx)
	if err != nil {
		return fmt.Errorf("handler NetworkID failed: %w, TxHash:%s", err, txHash)
	}

	tx, err := types.SignTx(
		types.NewTx(&types.LegacyTx{
			Nonce: nonce,
			// recepient address
			To: &toAddress,
			// wei(10^18)
			Value: setting.refundAmount,
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
		return fmt.Errorf("handler SignTx failed: %w, TxHash:%s", err, txHash)
	}

	if err := c.SendTransaction(ctx, tx); err != nil {
		return fmt.Errorf("handler SendTransaction failed: %w, TxHash:%s", err, txHash)
	}

	return nil
}
