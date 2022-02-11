package main

import (
	"log"

	"github.com/ethereum/go-ethereum/ethclient"
)

func main() {
	client, err := ethclient.Dial("https://prod-testnet.prod.findora.org:8545")
	if err != nil {
		log.Fatal(err)
	}

	_ = client
}
