package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
)

type Config struct {
	Server        *Server        `json:"server"`
	GasFeeService *GasFeeService `json:"gas_fee_service"`
}

type Server struct {
	// a timeout second while dialing to server do a websocket connection
	ServerDialTimeoutSec uint `json:"server_dial_timeout_sec"`
	// the websocket server address with port number
	ServerWSAddr string `json:"server_ws_addr"`
}

type GasFeeService struct {
	WorkerPoolSize      int `json:"worker_pool_size"`
	WorkerPoolWorkerNum int `json:"worker_pool_worker_num"`
	// Token address mapping to itself GasFeeRefundAmount
	RefundAmounts map[string]GasFeeRefundAmount `json:"refund_amounts"`
}

type GasFeeRefundAmount struct {
	Threshold uint `json:"threshold"`
	Refund    uint `json:"refund"`
}

func Load(cmd, filepath string) (*Config, error) {
	if cmd != "--config" {
		return nil, errors.New("config expecting a command --config along with the config filepath")
	}

	b, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("config read file failed: %w, filepath: %s", err, filepath)
	}

	c := &Config{}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("config json unmarshal failed: %w", err)
	}

	return c, nil
}
