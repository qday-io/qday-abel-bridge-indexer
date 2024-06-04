package types

import (
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// BITCOINTxIndexer defines the interface of custom bitcoin tx indexer.
type BITCOINTxIndexer interface {
	// ParseBlock parse bitcoin block tx
	ParseBlock(int64, int64) ([]*BitcoinTxParseResult, *BlockInfo, error)
	// LatestBlock get latest block height in the longest block chain.
	LatestBlock() (int64, error)
	// CheckConfirmations get tx detail info
	CheckConfirmations(txHash string) error

	GetRawTransactionVerbose(txHash string) (*TxInfo, error)
	BlockChainInfo() (*BlockChainInfo, error)
	GetRawTransaction(txHash *chainhash.Hash) (*TxInfo, error)
	GetBlockByHeight(height int64) (*BlockInfo, error)
}

type TxIndexer interface {
	BITCOINTxIndexer
	Stop()
}

type BitcoinTxParseResult struct {
	// from is l2 user address, by parse bitcoin get the address
	From []BitcoinFrom
	// to is listening address
	To string
	// value is from transfer amount
	Value int64
	// tx_id is the btc transaction id
	TxID string
	// tx_type is the type of the transaction, eg. "brc20_transfer","transfer"
	TxType string
	// index is the index of the transaction in the block
	Index int64
	// tos tx all to info
	Tos []BitcoinTo
}

type BitcoinFrom struct {
	Address string
}

type BitcoinTo struct {
	Address string
	Value   int64
}

type BlockChainInfo struct {
	Chain  string `json:"chain"`
	Blocks int64  `json:"blocks"`
	Data   any    `json:"data"`
}

type BlockInfo struct {
	Height    int64  `json:"height"`
	BlockHash string `json:"hash"`
	Time      int64  `json:"time"`
	Data      any    `json:"data"`
}

type TxInfo struct {
	Hash          string `json:"hash,omitempty"`
	Confirmations uint64 `json:"confirmations,omitempty"`
	Data          any    `json:"data"`
}
