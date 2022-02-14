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
		ServerWSAddress:      "ws://prod-testnet-us-west-2-sentry-000-public.prod.findora.org:8546",
	})

	_, err := c.Dial()
	assert.NoError(t, err)

	c = client.New(&config.Server{
		ServerDialTimeoutSec: 3,
		ServerWSAddress:      "not-exists-address",
	})

	_, err = c.Dial()
	assert.Error(t, err)
}
