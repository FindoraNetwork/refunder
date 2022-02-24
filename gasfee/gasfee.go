package gasfee

import (
	"crypto/ecdsa"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/FindoraNetwork/refunder/client"
	"github.com/FindoraNetwork/refunder/config"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type Service struct {
	client       client.Client
	stdoutlogger *log.Logger
	stderrlogger *log.Logger
	privateKey   *ecdsa.PrivateKey
	fromAddress  common.Address
	done         chan struct{}
	crawlerTick  *time.Ticker
	refundTick   *refundTicker
}

func New(c client.Client, conf *config.GasfeeService) (*Service, error) {
	privateKey, err := crypto.HexToECDSA(conf.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("new on crypto.HexToECDSA private key failed: %w", err)
	}

	publicKey, ok := privateKey.Public().(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("new on casting public key to ECDSA failed")
	}

	return &Service{
		client:       c,
		privateKey:   privateKey,
		fromAddress:  crypto.PubkeyToAddress(*publicKey),
		stdoutlogger: log.New(os.Stdout, "gasfeeService:", log.Lmsgprefix),
		stderrlogger: log.New(os.Stderr, "gasfeeService:", log.Lmsgprefix),
		done:         make(chan struct{}),
		crawlerTick:  time.NewTicker(time.Duration(conf.CrawleInEveryMinutes) * time.Minute),
		refundTick:   &refundTicker{period: 24 * time.Hour, at: conf.RefundEveryDayAt},
	}, nil
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
func (s *Service) Start() error {
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
	return nil
}

// Close stops the fork out goroutines from Start method
func (s *Service) Close() {
	s.crawlerTick.Stop()
	s.refundTick.timer.Stop()
	close(s.done)
}

func (s *Service) refunder() error {
	_, err := s.client.DialRPC()
	if err != nil {
		return fmt.Errorf("refunder client.DialRPC failed: %v", err)
	}

	return nil
}

func (s *Service) crawler() error {
	return nil
}
