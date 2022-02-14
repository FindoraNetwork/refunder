package client

import (
	"context"
	"fmt"
	"time"

	"github.com/FindoraNetwork/refunder/config"

	"github.com/ethereum/go-ethereum/ethclient"
)

type Client struct {
	config *config.Server
}

func New(config *config.Server) *Client {
	return &Client{
		config: config,
	}
}

func (c *Client) Dial() (*ethclient.Client, error) {
	dialTimeout, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(c.config.ServerDialTimeoutSec)*time.Second,
	)
	defer cancel()

	// ws://prod-testnet-us-west-2-sentry-000-public.prod.findora.org:8546
	// only works on sentry node
	client, err := ethclient.DialContext(dialTimeout, c.config.ServerWSAddress)
	if err != nil {
		return nil, fmt.Errorf("ethclient.Dial failed: %w, config: %v", err, c.config)
	}

	return client, nil
}
