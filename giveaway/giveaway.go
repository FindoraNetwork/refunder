package giveaway

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/FindoraNetwork/refunder/client"
	"github.com/FindoraNetwork/refunder/config"
	"github.com/FindoraNetwork/refunder/internal/workerpool"
	"github.com/gorilla/websocket"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type Service struct {
	client           client.Client
	wp               *workerpool.WorkerPool
	eventLogPoolSize int
	done             chan struct{}

	stdoutlogger *log.Logger
	stderrlogger *log.Logger

	subscribeTimeout    time.Duration
	handlerTotalTimeout time.Duration

	filterQuery ethereum.FilterQuery

	privateKey       *ecdsa.PrivateKey
	fromAddress      common.Address
	fixedGiveawayWei *big.Int
}

func New(c client.Client, conf *config.GiveawayService) (*Service, error) {
	privateKey, err := crypto.HexToECDSA(conf.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("new on crypto.HexToECDSA private key failed: %w", err)
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
		stdoutlogger:        log.New(os.Stdout, "giveawayService: ", log.Lmsgprefix),
		stderrlogger:        log.New(os.Stderr, "giveawayService: ", log.Lmsgprefix),
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
		privateKey:       privateKey,
		fromAddress:      crypto.PubkeyToAddress(*publicKey),
		fixedGiveawayWei: big.NewInt(0).SetUint64(conf.FixedGiveawayWei),
	}

	if err := s.Start(); err != nil {
		return nil, fmt.Errorf("new on starting service failed: %w", err)
	}

	return s, nil
}

// Start fork out a goroutine to listen to specific event log which is defined in filterQuery field then bypass into the handler
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
				// we have to handle event log one by one,
				// because current Findora Network has not implemented memory storage,
				// so getting any pending state is meanless,
				// and we have to maintain the nonce ourself.
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
		return fmt.Errorf("handler receive not expecting format on topics: %v, tx_hash:%s", vlog.Topics, txHash)
	}

	toAddress := common.BytesToAddress(common.TrimLeftZeroes(vlog.Topics[2].Bytes()))
	blockNumber := big.NewInt(0).SetUint64(vlog.BlockNumber)

	c, err := s.client.Dial()
	if err != nil {
		return fmt.Errorf("handler client dialing failed: %v, tx_hash:%s", err, txHash)
	}
	defer c.Close()

	toBalance, err := c.BalanceAt(ctx, toAddress, blockNumber)
	if err != nil {
		return fmt.Errorf("handler toAddress BalanceAt failed: %w, tx_hash:%s, toAddress:%v", err, txHash, toAddress)
	}

	toNonce, err := c.NonceAt(ctx, toAddress, blockNumber)
	if err != nil {
		return fmt.Errorf("handler toAddress NonceAt failed: %w, tx_hash:%s, toAddress:%v", err, txHash, toAddress)
	}

	s.stdoutlogger.Printf(`handler receiving:
to_address:	 %v 
to_balance:      %v
to_nonce:	 %v
block_number:	 %v
fix_giveaway:	 %v
tx_hash:	 %v
`,
		toAddress,
		toBalance,
		toNonce,
		blockNumber,
		s.fixedGiveawayWei,
		txHash,
	)

	if toBalance.Cmp(big.NewInt(0)) != 0 && toNonce != 0 {
		return ErrNotEligible
	}

	nonce, err := c.PendingNonceAt(ctx, s.fromAddress)
	if err != nil {
		return fmt.Errorf("handler PendingNonceAt failed: %w, tx_hash:%s", err, txHash)
	}

	gasPrice, err := c.SuggestGasPrice(ctx)
	if err != nil {
		return fmt.Errorf("handler SuggestGasPrice failed: %w, tx_hash:%s", err, txHash)
	}

	chainID, err := c.NetworkID(ctx)
	if err != nil {
		return fmt.Errorf("handler NetworkID failed: %w, tx_hash:%s", err, txHash)
	}

	tx, err := types.SignTx(
		types.NewTx(&types.LegacyTx{
			Nonce: nonce + 1,
			// recepient address
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
		return fmt.Errorf("handler SignTx failed: %w, tx_hash:%s", err, txHash)
	}

	if err := c.SendTransaction(ctx, tx); err != nil {
		return fmt.Errorf("handler SendTransaction failed: %w, tx_hash:%s", err, txHash)
	}

	return nil
}
