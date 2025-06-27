package _interface

import (
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/qday-io/qday-abel-bridge-indexer/internal/model"
)

// BitcoinTxIndexer defines the interface of custom bitcoin tx indexer.
type BitcoinTxIndexer interface {
	// ParseBlock parse bitcoin block tx
	ParseBlock(int64, int64) ([]*model.BitcoinTxParseResult, *model.BlockInfo, error)
	// LatestBlock get latest block height in the longest block chain.
	LatestBlock() (int64, error)
	// CheckConfirmations get tx detail info
	CheckConfirmations(txHash string) error

	GetRawTransactionVerbose(txHash string) (*model.TxInfo, error)
	BlockChainInfo() (*model.BlockChainInfo, error)
	GetRawTransaction(txHash *chainhash.Hash) (*model.TxInfo, error)
	GetBlockByHeight(height int64) (*model.BlockInfo, error)
}

type TxIndexer interface {
	BitcoinTxIndexer
	Stop()
}
