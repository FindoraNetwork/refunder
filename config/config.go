package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
)

type Config struct {
	// Server is the configuration for dialing to the EVM Websocket server
	Server *Server `json:"server"`
	// GiveawayService is the configuration for Type 1: Token Incentive / Giveaway service
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
	// FixedGiveawayWei is the constant amount of token to do the incentive
	// Like 0.003 FRA = 30000000000000000 wei
	FixedGiveawayWei *big.Int `json:"fixed_giveaway_wei"`
	// MaxCapWei is the total maximum incentive amount in Wei
	// Like 20000 FRA = 20000000000000000000000 wei
	MaxCapWei *big.Int `json:"max_cap_wei"`
	// TokenAddresses is the address of tokens gonna to listen to incentive
	TokenAddresses []string `json:"token_addresses"`
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
