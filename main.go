package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/FindoraNetwork/refunder/config"

	"github.com/ethereum/go-ethereum/ethclient"
)

func main() {
	config, err := config.Load(os.Args[1], os.Args[2])
	if err != nil {
		log.Fatalf("readConfig failed: %v", err)
	}

	dialTimeout, cancel := context.WithTimeout(context.Background(), time.Duration(config.ServerDialTimeoutSec)*time.Second)
	defer cancel()

	// ws://prod-testnet-us-west-2-sentry-000-public.prod.findora.org:8546
	// only works on sentry node
	client, err := ethclient.DialContext(dialTimeout, config.ServerWSAddr)
	if err != nil {
		log.Fatalf("ethclient.Dial failed: %v, config: %v", err, config)
	}

	_ = client
}
