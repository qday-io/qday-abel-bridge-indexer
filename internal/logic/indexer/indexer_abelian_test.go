package indexer

import (
	"encoding/json"
	"testing"

	"github.com/b2network/b2-indexer/config"
	logger "github.com/b2network/b2-indexer/pkg/log"
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

	txs, block, err := b.ParseBlock(304092, 0)
	if err != nil {
		t.Fatal(err)
		return
	}

	t.Logf("txs: %v block: %v", txs[0], block)

	bs, _ := json.Marshal(txs[0].Tos)
	t.Log(string(bs))
}
