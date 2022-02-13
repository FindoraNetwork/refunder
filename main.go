package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/FindoraNetwork/refunder/config"
	"github.com/FindoraNetwork/refunder/gasfee"

	"github.com/ethereum/go-ethereum/ethclient"
)

const help = `
Usage refunder [OPTION]... [FILE]...
Must need a Config File specified by --config option

Mandatory arguments to long options.
--config    specific the config file path"
`

func main() {
	if len(os.Args) <= 1 {
		log.Fatal(help)
	}

	config, err := config.Load(os.Args[1], os.Args[2])
	if err != nil {
		log.Fatalf("readConfig failed: %v", err)
	}

	dialTimeout, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(config.Server.ServerDialTimeoutSec)*time.Second,
	)
	defer cancel()

	// ws://prod-testnet-us-west-2-sentry-000-public.prod.findora.org:8546
	// only works on sentry node
	client, err := ethclient.DialContext(dialTimeout, config.Server.ServerWSAddr)
	if err != nil {
		log.Fatalf("ethclient.Dial failed: %v, config: %v", err, config)
	}

	gasfeeSvc, err := gasfee.New(client, config.GasFeeService)
	if err != nil {
		log.Fatalf("gasfee new service failed: %v, config: %v", err, config)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	<-c
	gasfeeSvc.Close()
}
