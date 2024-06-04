package indexer

import (
	"testing"

	"github.com/b2network/b2-indexer/internal/config"
	logger "github.com/b2network/b2-indexer/pkg/log"
)

func init2() *AbelianIndexer {
	bitcoinCfg, err := config.LoadBitcoinConfig("./")
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

	txs, block, err := b.ParseBlock(286934, 0)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("txs: %v block: %v", txs[0], block)

}
