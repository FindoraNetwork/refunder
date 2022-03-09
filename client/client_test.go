package client_test

import (
	"testing"

	"github.com/FindoraNetwork/refunder/client"
	"github.com/FindoraNetwork/refunder/config"

	"github.com/stretchr/testify/assert"
)

func Test_Client(t *testing.T) {
	c := client.New(&config.Server{
		ServerDialTimeoutSec: 3,
		ServerWSAddress:      "ws://prod-testnet-us-west-2-sentry-001-public.prod.findora.org:8546",
		ServerRPCAddress:     "https://prod-testnet.prod.findora.org:8545",
	})

	_, err := c.DialWS()
	assert.NoError(t, err)
	_, err = c.DialRPC()
	assert.NoError(t, err)

	c = client.New(&config.Server{
		ServerDialTimeoutSec: 3,
		ServerWSAddress:      "not-exists-address",
		ServerRPCAddress:     "not-exists-address",
	})

	gc, err := c.DialWS()
	assert.Error(t, err)
	assert.Nil(t, gc)
	gc, err = c.DialRPC()
	assert.Error(t, err)
	assert.Nil(t, gc)
}
