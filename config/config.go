package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"time"
)

type Config struct {
	// Server is the configuration for dialing to the EVM Websocket server
	Server *Server `json:"server"`
	// GiveawayService is the configuration for Type 1: Token Incentive / Giveaway service
	GiveawayService *GiveawayService `json:"giveaway_service"`
	// GasfeeService is the configuration for Type 2: Gas Refund
	GasfeeService *GasfeeService `json:"gasfee_service"`
}

type Server struct {
	// a timeout second while dialing to server do a websocket connection
	ServerDialTimeoutSec uint `json:"server_dial_timeout_sec"`
	// the websocket server addresses with port number
	ServerWSAddresses []string `json:"server_ws_addresses"`
	// the http server addresses with port number
	ServerRPCAddresses []string `json:"server_rpc_addresses"`
	// while RefundWithDynamicGasPrice in GasfeeService is true, this is a MUST filled field
	DynamicGasPriceRPCAddress string `json:"dynamic_gas_price_rpc_address"`
}

type GasfeeService struct {
	// IsEnable is a switch to enable this service or not
	IsEnable bool `json:"is_enable"`
	// PrivateKey for the founding source
	PrivateKey string `json:"-"`
	// CrawleInEveryMinutes specific a time period to crawle the gate.io information
	CrawleInEveryMinutes uint `json:"crawle_in_every_minutes"`
	// RefundEveryDayAt specific a time in RFC 3339 format which takes the HH:MM:SS only
	// and will using 24 hours as it's period
	RefundEveryDayAt time.Time `json:"refund_every_day_at"`
	// RefunderTotalTimeoutSec is the timeout second for all operations in the refunder function
	RefunderTotalTimeoutSec uint `json:"refunder_total_timeout_sec"`
	// RefunderStartBlockNumber defines the the FilterQuery.FromBlock on the first time start up
	RefunderStartBlockNumber uint64 `json:"refunder_start_block_number"`
	// RefunderScrapBlockStep is an interval scale of the FilterQuery.ToBlock should be while querying the event logs
	RefunderScrapBlockStep int `json:"refunder_scrap_block_step"`
	// CrawlerTotalTimeoutSec is the timeout second for all operations in the crawler function
	CrawlerTotalTimeoutSec uint `json:"crawler_total_timeout_sec"`
	// RefundThreshold defines the transaction refunding threshold
	// In USDT as unit currently
	RefundThreshold *big.Float `json:"refund_threshold"`
	// RefundMaxCapWei is the total maximum incentive amount in wei
	// Like 20000 FRA = 20000000000000000000000 wei
	RefundMaxCapWei *big.Int `json:"refund_max_cap_wei"`
	// IsUsingDynamicGasPrice enables the usage of XXX form 2
	IsUsingDynamicGasPrice bool `json:"is_using_dynamic_gas_price"`
	// RefundMaxUsdtEach limits each refunding FRA token should not be over the specific USDT price
	RefundMaxUsdtEach *big.Float `json:"refund_max_usdt_each"`
	// RefundBaseRateWei is the base rate XXX form 1 from the readme in wei if IsUsingDynamicGasPrice == false
	RefundBaseRateWei *big.Float `json:"refund_base_rate_wei"`
	// RefundedWeiFilepath stores the current refunded wei information
	RefundedWeiFilepath string `json:"refunded_wei_filepath"`
	// RefundedListFilepath stores the refunded addresses as a json array format to avoid multiple refunding
	RefundedListFilepath string `json:"refunded_list_filepath"`
	// Numerator is the target network name of the price pair
	Numerator CurrencyPair `json:"numerator"`
	// Denominator is the FRA network name of the price pair
	Denominator CurrencyPair `json:"denominator"`
	// CurrentBlockNumberFilepath stores the current served block high information
	CurrentBlockNumberFilepath string `json:"current_block_number_filepath"`
	// CrawlingAddress is the target address to crawle
	CrawlingAddress string `json:"crawling_address"`
	// CrawlingMapper defines the crawling target and its own settings
	// example:
	// "FRA_USDT": {
	//	"chain_id": 2153,
	//	"price_kind": 0
	//      ...
	// }
	CrawlingMapper map[CurrencyPair]*CrawlingMate `json:"crawling_mapper"`
}

type (
	CurrencyPair string
	PriceKind    int
)

const (
	// 0
	Highest = PriceKind(iota)
	// 1
	Lowest
)

type CrawlingMate struct {
	// PriceKind defines which kind of price will be stored
	PriceKind PriceKind `json:"price_kind"`
	// Decimal is the crawling target currency decimal number
	Decimal int `json:"decimal"`
	// TokenAddress is the address of the target token
	TokenAddress string `json:"token_address"`
}

type GiveawayService struct {
	// IsEnable is a switch to enable this service or not
	IsEnable bool `json:"is_enable"`
	// PrivateKey for the founding source
	PrivateKey string `json:"-"`
	// HandlerTotalTimeoutSec is the timeout second for all operations in the handle function
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
	// CurrentGaveWeiFilepath stores the current gave out wei information
	CurrentGaveWeiFilepath string `json:"current_gave_wei_filepath"`
}

const (
	envGiveawayServicePrivateKey = "GIVEAWAY_SERVICE_PK"
	envGasfeeServicePrivateKey   = "GASFEE_SERVICE_PK"
)

// Load simply loading the config from a json file which is specificed
// and read the private keys from the env
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

	if c.GiveawayService != nil {
		c.GiveawayService.PrivateKey = os.Getenv(envGiveawayServicePrivateKey)
	}

	if c.GasfeeService != nil {
		c.GasfeeService.PrivateKey = os.Getenv(envGasfeeServicePrivateKey)
	}

	return c, nil
}
