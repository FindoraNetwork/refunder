package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/FindoraNetwork/refunder/client"
	"github.com/FindoraNetwork/refunder/config"
	"github.com/FindoraNetwork/refunder/gasfee"
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

	client := client.New(config.Server)

	gasfeeSvc, err := gasfee.New(client, config.GasFeeService)
	if err != nil {
		log.Fatalf("gasfee new service failed: %v, config: %v", err, config)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	<-c
	gasfeeSvc.Close()
}
