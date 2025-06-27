package config

import (
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/caarlos0/env/v6"
)

// AppConfig 包含所有应用配置
type AppConfig struct {
	// Indexer 配置
	RootDir  string `env:"INDEXER_ROOT_DIR"`
	LogLevel string `env:"INDEXER_LOG_LEVEL" envDefault:"info"`
	// "console","json"
	LogFormat               string `env:"INDEXER_LOG_FORMAT" envDefault:"console"`
	DatabaseSource          string `env:"INDEXER_DATABASE_SOURCE" envDefault:"postgres://postgres:postgres@127.0.0.1:5432/b2-indexer"`
	DatabaseMaxIdleConns    int    `env:"INDEXER_DATABASE_MAX_IDLE_CONNS" envDefault:"10"`
	DatabaseMaxOpenConns    int    `env:"INDEXER_DATABASE_MAX_OPEN_CONNS" envDefault:"20"`
	DatabaseConnMaxLifetime int    `env:"INDEXER_DATABASE_CONN_MAX_LIFETIME" envDefault:"3600"`

	// Bitcoin 配置
	NetworkName string `env:"BITCOIN_NETWORK_NAME"`
	// RPCHost defines the bitcoin rpc host
	RPCHost string `env:"BITCOIN_RPC_HOST"`
	// RPCPort defines the bitcoin rpc port
	RPCPort string `env:"BITCOIN_RPC_PORT"`
	// RPCUser defines the bitcoin rpc user
	RPCUser string `env:"BITCOIN_RPC_USER"`
	// RPCPass defines the bitcoin rpc password
	RPCPass string `env:"BITCOIN_RPC_PASS"`
	// DisableTLS defines the bitcoin whether tls is required
	DisableTLS bool `env:"BITCOIN_DISABLE_TLS" envDefault:"true"`
	// WalletName defines the bitcoin wallet name
	WalletName string `env:"BITCOIN_WALLET_NAME"`
	// EnableIndexer defines whether to enable the indexer
	EnableIndexer bool `env:"BITCOIN_ENABLE_INDEXER"`
	// IndexerListenAddress defines the address to listen on
	IndexerListenAddress string `env:"BITCOIN_INDEXER_LISTEN_ADDRESS"`
	// IndexerListenTargetConfirmations defines the number of confirmations to listen on
	IndexerListenTargetConfirmations uint64 `env:"BITCOIN_INDEXER_LISTEN_TARGET_CONFIRMATIONS" envDefault:"1"`

	// Bridge 配置
	Bridge BridgeConfig
}

// Config is the global config.
type Config struct {
	// The root directory for all data.
	RootDir  string `env:"INDEXER_ROOT_DIR"`
	LogLevel string `env:"INDEXER_LOG_LEVEL" envDefault:"info"`
	// "console","json"
	LogFormat               string `env:"INDEXER_LOG_FORMAT" envDefault:"console"`
	DatabaseSource          string `env:"INDEXER_DATABASE_SOURCE" envDefault:"postgres://postgres:postgres@127.0.0.1:5432/b2-indexer"`
	DatabaseMaxIdleConns    int    `env:"INDEXER_DATABASE_MAX_IDLE_CONNS" envDefault:"10"`
	DatabaseMaxOpenConns    int    `env:"INDEXER_DATABASE_MAX_OPEN_CONNS" envDefault:"20"`
	DatabaseConnMaxLifetime int    `env:"INDEXER_DATABASE_CONN_MAX_LIFETIME" envDefault:"3600"`
}

// BitcoinConfig defines the bitcoin config
type BitcoinConfig struct {
	// NetworkName defines the bitcoin network name
	NetworkName string `env:"BITCOIN_NETWORK_NAME"`
	// RPCHost defines the bitcoin rpc host
	RPCHost string `env:"BITCOIN_RPC_HOST"`
	// RPCPort defines the bitcoin rpc port
	RPCPort string `env:"BITCOIN_RPC_PORT"`
	// RPCUser defines the bitcoin rpc user
	RPCUser string `env:"BITCOIN_RPC_USER"`
	// RPCPass defines the bitcoin rpc password
	RPCPass string `env:"BITCOIN_RPC_PASS"`
	// DisableTLS defines the bitcoin whether tls is required
	DisableTLS bool `env:"BITCOIN_DISABLE_TLS" envDefault:"true"`
	// WalletName defines the bitcoin wallet name
	WalletName string `env:"BITCOIN_WALLET_NAME"`
	// EnableIndexer defines whether to enable the indexer
	EnableIndexer bool `env:"BITCOIN_ENABLE_INDEXER"`
	// IndexerListenAddress defines the address to listen on
	IndexerListenAddress string `env:"BITCOIN_INDEXER_LISTEN_ADDRESS"`
	// IndexerListenTargetConfirmations defines the number of confirmations to listen on
	IndexerListenTargetConfirmations uint64 `env:"BITCOIN_INDEXER_LISTEN_TARGET_CONFIRMATIONS" envDefault:"1"`
	// Bridge defines the bridge config
	Bridge BridgeConfig
}

type BridgeConfig struct {
	// EthRPCURL defines the ethereum rpc url, b2 rollup rpc
	EthRPCURL string `env:"BITCOIN_BRIDGE_ETH_RPC_URL"`
	// EthPrivKey defines the invoke ethereum private key
	EthPrivKey string `env:"BITCOIN_BRIDGE_ETH_PRIV_KEY"`
	// ContractAddress defines the l1 -> l2 bridge contract address
	ContractAddress string `env:"BITCOIN_BRIDGE_CONTRACT_ADDRESS"`
	// ABI defines the l1 -> l2 bridge contract abi
	ABI string `env:"BITCOIN_BRIDGE_ABI"`

	// AAB2PI get pubkey by btc address
	AAB2PI string `env:"BITCOIN_BRIDGE_AA_B2_API"`

	// GasPriceMultiple defines the gas price multiple, TODO: temp fix, base gas_price * n
	GasPriceMultiple int64 `env:"BITCOIN_BRIDGE_GAS_PRICE_MULTIPLE" envDefault:"2"`
	// B2ExplorerURL defines the b2 explorer url, TODO: temp use explorer gas prices
	B2ExplorerURL string `env:"BITCOIN_BRIDGE_B2_EXPLORER_URL"`
	// EnableListener defines whether to enable the listener
	EnableWithdrawListener bool `env:"BITCOIN_BRIDGE_WITHDRAW_ENABLE_LISTENER"`
	// Deposit defines the deposit event hash
	Deposit string `env:"BITCOIN_BRIDGE_DEPOSIT"`
	// Withdraw defines the withdraw event hash
	Withdraw string `env:"BITCOIN_BRIDGE_WITHDRAW"`
	// UnisatApiKey defines unisat api_key
	UnisatAPIKey string `env:"BITCOIN_BRIDGE_UNISAT_API_KEY"`
	// PublicKeys defines signer publickey
	PublicKeys []string `env:"BITCOIN_BRIDGE_PUBLICKEYS"`
	// TimeInterval defines withdraw time interval
	TimeInterval int64 `env:"BITCOIN_BRIDGE_TIME_INTERVAL"`
	// MultisigNum defines withdraw multisig number
	MultisigNum int `env:"BITCOIN_BRIDGE_MULTISIG_NUM"`
	// EnableRollupListener defines rollup index server
	EnableRollupListener bool `env:"BITCOIN_BRIDGE_ROLLUP_ENABLE_LISTENER"`
}

const (
	BitcoinConfigEnvPrefix = "BITCOIN"
	AppConfigEnvPrefix     = "APP"
)

// LoadAppConfig 统一加载所有配置
func LoadAppConfig() (*AppConfig, error) {
	config := AppConfig{}

	// 直接使用环境变量加载配置
	if err := env.Parse(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// LoadConfig 加载应用配置（保持向后兼容）
func LoadConfig() (*Config, error) {
	config := Config{}

	// 直接使用环境变量加载配置
	if err := env.Parse(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// LoadBitcoinConfig 加载 Bitcoin 配置（保持向后兼容）
func LoadBitcoinConfig() (*BitcoinConfig, error) {
	config := BitcoinConfig{}

	// 直接使用环境变量加载配置
	if err := env.Parse(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// ChainParams get chain params by network name
func ChainParams(network string) *chaincfg.Params {
	switch network {
	case chaincfg.MainNetParams.Name:
		return &chaincfg.MainNetParams
	case chaincfg.TestNet3Params.Name:
		return &chaincfg.TestNet3Params
	case chaincfg.SigNetParams.Name:
		return &chaincfg.SigNetParams
	case chaincfg.SimNetParams.Name:
		return &chaincfg.SimNetParams
	case chaincfg.RegressionNetParams.Name:
		return &chaincfg.RegressionNetParams
	default:
		return &chaincfg.TestNet3Params
	}
}

func DefaultConfig() *Config {
	return &Config{
		RootDir:  "",
		LogLevel: "info",
	}
}

func DefaultBitcoinConfig() *BitcoinConfig {
	return &BitcoinConfig{
		EnableIndexer: false,
		NetworkName:   "testnet3",
		RPCHost:       "https://bitcoin-testnet.drpc.org",
		RPCUser:       "",
		RPCPass:       "",
		RPCPort:       "",
	}
}
