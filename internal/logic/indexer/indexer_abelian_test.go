package indexer

import (
	"encoding/json"
	"testing"

	"github.com/b2network/b2-indexer/config"
	logger "github.com/b2network/b2-indexer/pkg/log"
	"github.com/stretchr/testify/assert"
)

func init2() *AbelianIndexer {
	bitcoinCfg, err := config.LoadBitcoinConfig("../../../cmd/")
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

func TestAbelianIndexer_ParseBlock(t *testing.T) {
	b := init2()

	txs, block, err := b.ParseBlock(373168, 0)

	assert.NoError(t, err)

	t.Logf("txs: %v block: %v", txs, block)

	if len(txs) > 0 {
		bs, _ := json.Marshal(txs[0].Tos)
		t.Log(string(bs))
	}
}

func TestAbelianIndexer_BlockChainInfo(t *testing.T) {

	b := init2()
	resp, err := b.BlockChainInfo()
	assert.NoError(t, err)

	t.Log(resp.Blocks)

}
