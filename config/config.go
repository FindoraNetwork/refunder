package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
)

type Config struct {
	// a timeout second while dialing to server do a websocket connection
	ServerDialTimeoutSec uint
	// the websocket server address with port number
	ServerWSAddr string
	// a maximun refund limitation
	MaxRefund uint64
	// TODO: need to know what this field mean
	BridgeTokenAddr string
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
