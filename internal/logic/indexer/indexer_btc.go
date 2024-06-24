package indexer

import (
	"errors"
	"fmt"

	"github.com/b2network/b2-indexer/config"
	"github.com/b2network/b2-indexer/internal/model"
	"github.com/b2network/b2-indexer/internal/types"
	"github.com/b2network/b2-indexer/pkg/log"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// BtcIndexer bitcoin indexer, parse and forward data
type BtcIndexer struct {
	client              *rpcclient.Client // call bitcoin rpc client
	chainParams         *chaincfg.Params  // bitcoin network params, e.g. mainnet, testnet, etc.
	listenAddress       btcutil.Address   // need listened bitcoin address
	targetConfirmations uint64
	logger              log.Logger
}

// NewBitcoinIndexer new bitcoin indexer
func NewBitcoinIndexer(
	log log.Logger,
	ctx *model.Context,
	listenAddress string,
	targetConfirmations uint64,
) (types.TxIndexer, error) {

	bitcoinCfg := ctx.BitcoinConfig
	bitcoinParam := config.ChainParams(bitcoinCfg.NetworkName)
	// check listenAddress
	address, err := btcutil.DecodeAddress(listenAddress, bitcoinParam)

	if err != nil {
		return nil, fmt.Errorf("%w:%s", ErrDecodeListenAddress, err.Error())
	}

	bclient, err := rpcclient.New(&rpcclient.ConnConfig{
		Host:         bitcoinCfg.RPCHost + ":" + bitcoinCfg.RPCPort,
		User:         bitcoinCfg.RPCUser,
		Pass:         bitcoinCfg.RPCPass,
		HTTPPostMode: true,                  // Bitcoin core only supports HTTP POST mode
		DisableTLS:   bitcoinCfg.DisableTLS, // Bitcoin core does not provide TLS by default
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("ailed to create bitcoin client:%v", err.Error())
	}

	return &BtcIndexer{
		logger:              log,
		client:              bclient,
		chainParams:         bitcoinParam,
		listenAddress:       address,
		targetConfirmations: targetConfirmations,
	}, nil
}

func (b *BtcIndexer) Stop() {
	b.client.Shutdown()
}

// ParseBlock parse block data by block height
// NOTE: Currently, only transfer transactions are supported.
func (b *BtcIndexer) ParseBlock(height int64, txIndex int64) ([]*types.BitcoinTxParseResult, *types.BlockInfo, error) {
	blockResult, err := b.GetBlockByHeight(height)
	if err != nil {
		return nil, nil, err
	}

	MsgBlock, ok := blockResult.Data.(*wire.MsgBlock)
	if !ok {
		return nil, nil, fmt.Errorf("btc block convert error")
	}

	blockParsedResult := make([]*types.BitcoinTxParseResult, 0)
	for k, v := range MsgBlock.Transactions {
		if int64(k) < txIndex {
			continue
		}

		b.logger.Debugw("parse block", "k", k, "height", height, "txIndex", txIndex, "tx", v.TxHash().String())

		parseTxs, err := b.parseTx(v, k)
		if err != nil {
			return nil, nil, err
		}
		b.logger.Infof("parse block:height=%v,txIndex=%v", height, k)

		if parseTxs != nil {
			blockParsedResult = append(blockParsedResult, parseTxs)
		}
	}

	block := types.BlockInfo{Time: MsgBlock.Header.Timestamp.Unix(), BlockHash: MsgBlock.BlockHash().String(), Height: height, Data: MsgBlock}
	return blockParsedResult, &block, nil
}

func (b *BtcIndexer) CheckConfirmations(hash string) error {
	txVerbose, err := b.GetRawTransactionVerbose(hash)
	if err != nil {
		return err
	}

	if txVerbose.Confirmations < b.targetConfirmations {
		return fmt.Errorf("%w, current confirmations:%d target confirmations: %d",
			ErrTargetConfirmations, txVerbose.Confirmations, b.targetConfirmations)
	}
	return nil
}

// parseTx parse transaction data
func (b *BtcIndexer) parseTx(txResult *wire.MsgTx, index int) (*types.BitcoinTxParseResult, error) {
	//listenAddress := false
	//var totalValue int64
	//tos := make([]types.BitcoinTo, 0)
	tos, totalValue, listenAddress, _ := b.parseToAddress(txResult.TxOut)
	if listenAddress {
		fromAddress, err := b.parseFromAddress(txResult.TxIn)
		if err != nil {
			return nil, fmt.Errorf("vin parse err:%w", err)
		}

		// TODO: temp fix, if from is listened address, continue
		if len(fromAddress) == 0 {
			b.logger.Warnw("parse from address empty or nonsupport tx type",
				"txId", txResult.TxHash().String(),
				"listenAddress", b.listenAddress.EncodeAddress())
			return nil, nil
		}

		return &types.BitcoinTxParseResult{
			TxID:   txResult.TxHash().String(),
			TxType: TxTypeTransfer,
			Index:  int64(index),
			Value:  totalValue,
			From:   fromAddress,
			To:     b.listenAddress.EncodeAddress(),
			Tos:    tos,
		}, nil
	}
	return nil, nil
}
func (b *BtcIndexer) parseToAddress(TxOut []*wire.TxOut) (toAddress []types.BitcoinTo, value int64, listenAddress bool, err error) {
	hasListenAddress := false
	var totalValue int64
	tos := make([]types.BitcoinTo, 0)

	for _, v := range TxOut {
		pkAddress, err := b.parseAddress(v.PkScript)
		if err != nil {
			if errors.Is(err, ErrParsePkScript) {
				continue
			}
			// null data
			if errors.Is(err, ErrParsePkScriptNullData) {
				continue
			}
			return nil, 0, false, err
		}
		parseTo := types.BitcoinTo{
			Address: pkAddress,
			Value:   v.Value,
		}
		tos = append(tos, parseTo)
		// if pk address eq dest listened address, after parse from address by vin prev tx
		if pkAddress == b.listenAddress.EncodeAddress() {
			hasListenAddress = true
			totalValue += v.Value
		}
	}

	return tos, totalValue, hasListenAddress, nil
}

// parseFromAddress from vin parse from address
// return all possible values parsed from address
// TODO: at present, it is assumed that it is a single from, and multiple from needs to be tested later
func (b *BtcIndexer) parseFromAddress(TxIn []*wire.TxIn) (fromAddress []types.BitcoinFrom, err error) {
	for _, vin := range TxIn {
		// get prev tx hash
		prevTxID := vin.PreviousOutPoint.Hash
		if prevTxID.String() == "0000000000000000000000000000000000000000000000000000000000000000" {
			return nil, nil
		}
		vinResult, err := b.GetRawTransaction(&prevTxID)
		if err != nil {
			return nil, fmt.Errorf("vin get raw transaction err:%w", err)
		}

		MsgTx, ok := vinResult.Data.(*btcutil.Tx)
		if !ok {
			return nil, fmt.Errorf("vin convert error")
		}

		if len(MsgTx.MsgTx().TxOut) == 0 {
			return nil, fmt.Errorf("vin txOut is null")
		}
		vinPKScript := MsgTx.MsgTx().TxOut[vin.PreviousOutPoint.Index].PkScript
		//  script to address
		vinPkAddress, err := b.parseAddress(vinPKScript)
		if err != nil {
			b.logger.Errorw("vin parse address", "error", err)
			if errors.Is(err, ErrParsePkScript) || errors.Is(err, ErrParsePkScriptNullData) {
				continue
			}
			return nil, err
		}

		fromAddress = append(fromAddress, types.BitcoinFrom{
			Address: vinPkAddress,
		})
	}
	return fromAddress, nil
}

// parseAddress from pkscript parse address
func (b *BtcIndexer) ParseAddress(pkScript []byte) (string, error) {
	return b.parseAddress(pkScript)
}

// parseNullData from pkscript parse null data
//
//lint:ignore U1000 Ignore unused function temporarily for debugging
func (b *BtcIndexer) parseNullData(pkScript []byte) (string, error) {
	pk, err := txscript.ParsePkScript(pkScript)
	if err != nil {
		return "", fmt.Errorf("%w:%s", ErrParsePkScript, err.Error())
	}
	if pk.Class() != txscript.NullDataTy {
		return "", fmt.Errorf("not null data type")
	}
	return pk.String(), nil
}

// parseAddress from pkscript parse address
func (b *BtcIndexer) parseAddress(pkScript []byte) (string, error) {
	pk, err := txscript.ParsePkScript(pkScript)
	if err != nil {
		return "", fmt.Errorf("%w:%s", ErrParsePkScript, err.Error())
	}

	if pk.Class() == txscript.NullDataTy {
		return "", ErrParsePkScriptNullData
	}

	//  encodes the script into an address for the given chain.
	pkAddress, err := pk.Address(b.chainParams)
	if err != nil {
		return "", fmt.Errorf("PKScript to address err:%w", err)
	}
	return pkAddress.EncodeAddress(), nil
}

// LatestBlock get latest block height in the longest block chain.
func (b *BtcIndexer) LatestBlock() (int64, error) {
	return b.client.GetBlockCount()
}

// BlockChainInfo get block chain info
func (b *BtcIndexer) BlockChainInfo() (*types.BlockChainInfo, error) {
	GetBlockChainInfoResult, err := b.client.GetBlockChainInfo()
	if err != nil {
		return nil, err
	}

	blockchainInfo := &types.BlockChainInfo{
		Chain:  GetBlockChainInfoResult.Chain,
		Blocks: int64(GetBlockChainInfoResult.Blocks),
		Data:   GetBlockChainInfoResult,
	}

	return blockchainInfo, nil
}

func (b *BtcIndexer) GetRawTransactionVerbose(hash string) (*types.TxInfo, error) {
	txHash, err := chainhash.NewHashFromStr(hash)
	if err != nil {
		return nil, err
	}
	txVerbose, err := b.client.GetRawTransactionVerbose(txHash)
	if err != nil {
		return nil, err
	}

	tx := &types.TxInfo{
		Hash:          hash,
		Confirmations: txVerbose.Confirmations,
		Data:          txVerbose,
	}

	return tx, nil
}

func (b *BtcIndexer) GetRawTransaction(txHash *chainhash.Hash) (*types.TxInfo, error) {
	txVerbose, err := b.client.GetRawTransaction(txHash)
	if err != nil {
		return nil, err
	}

	tx := &types.TxInfo{
		Hash:          txHash.String(),
		Confirmations: 0,
		Data:          txVerbose,
	}
	return tx, nil
}

// GetBlockByHeight returns a raw block from the server given its height
func (b *BtcIndexer) GetBlockByHeight(height int64) (*types.BlockInfo, error) {
	blockhash, err := b.client.GetBlockHash(height)
	if err != nil {
		return nil, err
	}
	msgBlock, err := b.client.GetBlock(blockhash)
	if err != nil {
		return nil, err
	}

	block := &types.BlockInfo{
		Height:    height,
		BlockHash: blockhash.String(),
		Time:      msgBlock.Header.Timestamp.Unix(),
		Data:      msgBlock,
	}

	return block, nil
}
