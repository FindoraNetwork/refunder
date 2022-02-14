package client

import (
	"context"
	"fmt"
	"time"

	"github.com/FindoraNetwork/refunder/config"

	"github.com/ethereum/go-ethereum/ethclient"
)

// Client is a wrapper of ethclient
// The goal is providing a simple way for services can do reconnecting
type Client struct {
	config *config.Server
}

// New returns a ethclient wrapper structure
func New(config *config.Server) *Client {
	return &Client{
		config: config,
	}
}

// Dial calls the ethclient.DialContext directly
func (c *Client) Dial() (*ethclient.Client, error) {
	dialTimeout, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(c.config.ServerDialTimeoutSec)*time.Second,
	)
	defer cancel()

	// ws://prod-testnet-us-west-2-sentry-000-public.prod.findora.org:8546
	// NOTE: Findora network only works on sentry node and no TLS supported
	client, err := ethclient.DialContext(dialTimeout, c.config.ServerWSAddress)
	if err != nil {
		return nil, fmt.Errorf("ethclient.Dial failed: %w, config: %v", err, c.config)
	}

	return client, nil
}
