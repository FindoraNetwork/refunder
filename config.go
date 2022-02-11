package main

import (
	"os"
	"strconv"
)

type config struct {
	ServerAddr      string
	MaxRefund       uint64
	BridgeTokenAddr string
}

const (
	EnvServerAddress      = "REFUNDER_SERVER_ADDRESS"
	EnvMaxRefund          = "REFUNDER_MAX_REFUND"
	EnvBridgeTokenAddress = "REFUNDER_BRIDGE_TOKEN_ADDRESS"
)

func readConfig() (*config, error) {
	maxRefund, err := strconv.ParseUint(os.Getenv(EnvMaxRefund), 10, 64)
	if err != nil {
		return nil, err
	}

	return &config{
		ServerAddr:      os.Getenv(EnvServerAddress),
		MaxRefund:       maxRefund,
		BridgeTokenAddr: os.Getenv(EnvBridgeTokenAddress),
	}, nil
}
