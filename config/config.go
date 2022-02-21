package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
)

type Config struct {
	// Server is the configuration for dialing to the EVM Websocket server
	Server *Server `json:"server"`
	// GasFeeService is the configuration for gas fee service
	GasFeeService   *GasFeeService   `json:"gas_fee_service"`
	GiveawayService *GiveawayService `json:"giveaway_service"`
}

type Server struct {
	// a timeout second while dialing to server do a websocket connection
	ServerDialTimeoutSec uint `json:"server_dial_timeout_sec"`
	// the websocket server address with port number
	ServerWSAddress string `json:"server_ws_address"`
}

type GiveawayService struct {
	// PrivateKey for the founding source
	PrivateKey string `json:"private_key"`
	// HandlerTotalTimeoutSec is the timeout second for all operations int the handle function
	HandlerTotalTimeoutSec uint `json:"handler_operations_timeout_sec"`
	// SubscripTimeoutSec is the timeout second for dialing and subscribing to the server
	SubscripTimeoutSec uint `json:"subscrip_timeout_sec"`
	// EventLogPoolSize is the size of the subscribed buffered channel
	EventLogPoolSize int `json:"event_log_pool_size"`
	// FixedGiveawayWei is the amount of token to incentive
	// Like 0.003 FRA = 30000000000000000 wei
	FixedGiveawayWei uint64 `json:"fixed_giveaway_wei"`
	// TokenAddresses is the address of tokens gonna to listen to incentive
	TokenAddresses []string `json:"token_addresses"`
}

type GasFeeService struct {
	// PrivateKey for the founding source
	PrivateKey string `json:"private_key"`
	// HandlerOperationsTimeoutSec is the timeout second for all operations of the handle function
	HandlerOperationsTimeoutSec uint `json:"handler_operations_timeout_sec"`
	// SubscripTimeoutSec is the timeout second for dialing and subscribing to the server
	SubscripTimeoutSec uint `json:"subscrip_timeout_sec"`
	// EventLogPoolSize is the size of the subscribed buffered channel
	EventLogPoolSize int `json:"event_log_pool_size"`
	// WorkerPoolSize is the size of the workerpool buffered channel
	WorkerPoolSize int `json:"worker_pool_size"`
	// WorkerPoolWorkerNum is the worker number in the workerpool
	WorkerPoolWorkerNum int `json:"worker_pool_worker_num"`
	// Token address mapping to itself GasFeeRefundAmount
	RefundAmounts map[string]*GasFeeRefundAmount `json:"refund_amounts"`
}

type GasFeeRefundAmount struct {
	// Threshold is the limitation to detect should we refound this event log or not
	// In logic if the transfering amount is smaller to this threshold then we skip the refund action
	Threshold int64 `json:"threshold"`
	// Refund is a fix amount to refund to the recepient
	Refund int64 `json:"refund"`
}

// Load simply loading the config from a json file which is specificed
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
