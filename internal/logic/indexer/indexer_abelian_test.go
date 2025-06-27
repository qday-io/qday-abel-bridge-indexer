package indexer

import (
	"testing"

	"github.com/qday-io/qday-abel-bridge-indexer/config"
	logger "github.com/qday-io/qday-abel-bridge-indexer/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init2() *AbelianIndexer {
	bitcoinCfg, err := config.LoadBitcoinConfig()
	if err != nil {
		panic(err)
	}

	return &AbelianIndexer{
		listenAddress:       bitcoinCfg.IndexerListenAddress,
		targetConfirmations: bitcoinCfg.IndexerListenTargetConfirmations,
		logger:              logger.NewNopLogger(),
		bitcoinCfg:          bitcoinCfg,
	}
}

func TestAbelianIndexer_Init(t *testing.T) {
	// 测试索引器初始化，不执行实际的网络调用
	b := init2()
	require.NotNil(t, b)
	assert.Equal(t, ":9090", b.listenAddress)
	assert.Equal(t, uint64(1), b.targetConfirmations)
}

func TestAbelianIndexer_Config(t *testing.T) {
	// 测试配置加载
	bitcoinCfg, err := config.LoadBitcoinConfig()
	require.NoError(t, err)
	assert.NotNil(t, bitcoinCfg)
	assert.Equal(t, "testnet3", bitcoinCfg.NetworkName)
}
