package gasfee

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/FindoraNetwork/refunder/config"
	"github.com/FindoraNetwork/refunder/internal/workerpool"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Service struct {
	client       *ethclient.Client
	wp           *workerpool.WorkerPool
	done         chan struct{}
	contractAddr string
	stdoutlogger *log.Logger
	stderrlogger *log.Logger
}

func New(client *ethclient.Client, config *config.GasFeeService) (*Service, error) {
	errLogger := log.New(os.Stderr, "gasfeeService", log.Lmsgprefix)

	addresses := make([]common.Address, 0, len(config.RefundAmounts))
	for addr := range config.RefundAmounts {
		addresses = append(addresses, common.HexToAddress(addr))
	}

	query := ethereum.FilterQuery{
		Addresses: addresses,
		// Topics:    [][]common.Hash{},
	}
	logs := make(chan types.Log)

	s := &Service{
		client:       client,
		stdoutlogger: log.New(os.Stdout, "gasfeeService", log.Lmsgprefix),
		stderrlogger: errLogger,
		wp: workerpool.New(
			workerpool.WithPoolSize(config.WorkerPoolSize),
			workerpool.WithWorkerNum(config.WorkerPoolWorkerNum),
			workerpool.WithLogger(errLogger),
		),
		done: make(chan struct{}),
	}

	sub, err := s.client.SubscribeFilterLogs(context.Background(), query, logs)
	if err != nil {
		return nil, fmt.Errorf("subscribe filter logs failed: %w", err)
	}

	go func() {
		for {
			select {
			case <-s.done:
				return
			case err := <-sub.Err():
				s.stderrlogger.Println(err)
			case vlog := <-logs:
				s.handler(vlog)
			}
		}
	}()

	return s, nil
}

func (s *Service) Close() {
	close(s.done)
}

func (s *Service) handler(vlog types.Log) {
	s.stdoutlogger.Println(vlog)
}
