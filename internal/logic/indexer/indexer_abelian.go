package indexer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

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

// AbelianIndexer bitcoin indexer, parse and forward data
type AbelianIndexer struct {
	client              *rpcclient.Client // call bitcoin rpc client
	chainParams         *chaincfg.Params  // bitcoin network params, e.g. mainnet, testnet, etc.
	listenAddress       btcutil.Address   // need listened bitcoin address
	targetConfirmations uint64
	ctx                 *model.Context
	logger              log.Logger
}

// NewAbelianIndexer new bitcoin indexer
func NewAbelianIndexer(
	log log.Logger,
	ctx *model.Context,
	chainParams *chaincfg.Params,
	listenAddress string,
	targetConfirmations uint64,
) (types.BITCOINTxIndexer, error) {
	// check listenAddress
	address, err := btcutil.DecodeAddress(listenAddress, chainParams)
	if err != nil {
		return nil, fmt.Errorf("%w:%s", ErrDecodeListenAddress, err.Error())
	}

	return &AbelianIndexer{
		logger:              log,
		client:              nil,
		chainParams:         chainParams,
		listenAddress:       address,
		ctx:                 ctx,
		targetConfirmations: targetConfirmations,
	}, nil
}

func (b *AbelianIndexer) newRequest(id string, method string, params []interface{}) (*http.Request, error) {
	jsonReq := &AbecJSONRPCRequest{
		JSONRPC: "1.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}
	jsonBody, err := json.Marshal(jsonReq)
	if err != nil {
		return nil, err
	}

	bitcoinCfg := b.ctx.BitcoinConfig
	//url := "https://testnet-rpc-00.abelian.info"
	url := bitcoinCfg.RPCHost + ":" + bitcoinCfg.RPCPort

	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.SetBasicAuth(bitcoinCfg.RPCUser, bitcoinCfg.RPCPass)

	return httpReq, nil
}

func (b *AbelianIndexer) getResponseFromChan(method string, params []interface{}) ([]byte, error) {
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	req, err := b.newRequest(id, method, params)
	if err != nil {
		return nil, err
	}

	client := http.DefaultClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		b.logger.Warnf("Response(%s): ERROR(%s)\n", id, err)
		return nil, err
	}

	respObj := &AbecJSONRPCResponse{}
	err = json.Unmarshal(body, respObj)
	if err != nil {
		return nil, err
	}

	errorStr := string(respObj.Error)
	if len(errorStr) > 0 && errorStr != "null" {
		return nil, fmt.Errorf("abec.%s: %s", method, respObj.Error)
	}

	return respObj.Result, nil
}

func (b *AbelianIndexer) Stop() {
	b.client.Shutdown()
}

// ParseBlock parse block data by block height
// NOTE: Currently, only transfer transactions are supported.
func (b *AbelianIndexer) ParseBlock(height int64, txIndex int64) ([]*types.BitcoinTxParseResult, *types.BlockInfo, error) {
	blockResult, err := b.GetBlockByHeight(height)
	if err != nil {
		return nil, nil, err
	}

	MsgBlock, ok := blockResult.Data.(AbecBlock)
	if !ok {
		return nil, nil, fmt.Errorf("btc block convert error")
	}

	blockParsedResult := make([]*types.BitcoinTxParseResult, 0)
	for k, v := range MsgBlock.RawTxs {
		if int64(k) < txIndex {
			continue
		}

		b.logger.Debugw("parse block", "k", k, "height", height, "txIndex", txIndex, "tx", v.TxHash)

		parseTxs, err := b.parseTx(v, k)
		if err != nil {
			return nil, nil, err
		}
		b.logger.Infof("parse block:height=%v,txIndex=%v", height, k)

		if parseTxs != nil {
			blockParsedResult = append(blockParsedResult, parseTxs)
		}
	}

	return blockParsedResult, blockResult, nil
}

func (b *AbelianIndexer) CheckConfirmations(hash string) error {
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
func (b *AbelianIndexer) parseTx(txResult *AbecTx, index int) (*types.BitcoinTxParseResult, error) {
	//listenAddress := false
	//var totalValue int64
	//tos := make([]types.BitcoinTo, 0)

	//
	//tos, totalValue, listenAddress, _ := b.parseToAddress(txResult.TxOut)
	//if listenAddress {
	//	fromAddress, err := b.parseFromAddress(txResult.TxIn)
	//	if err != nil {
	//		return nil, fmt.Errorf("vin parse err:%w", err)
	//	}
	//
	//	// TODO: temp fix, if from is listened address, continue
	//	if len(fromAddress) == 0 {
	//		b.logger.Warnw("parse from address empty or nonsupport tx type",
	//			"txId", txResult.TxHash().String(),
	//			"listenAddress", b.listenAddress.EncodeAddress())
	//		return nil, nil
	//	}
	//
	//	return &types.BitcoinTxParseResult{
	//		TxID:   txResult.TxHash().String(),
	//		TxType: TxTypeTransfer,
	//		Index:  int64(index),
	//		Value:  totalValue,
	//		From:   fromAddress,
	//		To:     b.listenAddress.EncodeAddress(),
	//		Tos:    tos,
	//	}, nil
	//}
	return nil, nil
}
func (b *AbelianIndexer) parseToAddress(TxOut []*wire.TxOut) (toAddress []types.BitcoinTo, value int64, listenAddress bool, err error) {
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
func (b *AbelianIndexer) parseFromAddress(TxIn []*wire.TxIn) (fromAddress []types.BitcoinFrom, err error) {
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

		MsgTx, ok := vinResult.Data.(AbecTx)
		if !ok {
			return nil, fmt.Errorf("vin convert error")
		}

		if len(MsgTx.Vout) == 0 {
			return nil, fmt.Errorf("txOut is null")
		}
		vinPKScript := MsgTx.Vout[vin.PreviousOutPoint.Index].Script
		//  script to address
		vinPkAddress, err := b.parseAddress([]byte(vinPKScript))
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
func (b *AbelianIndexer) ParseAddress(pkScript []byte) (string, error) {
	return b.parseAddress(pkScript)
}

// parseNullData from pkscript parse null data
//
//lint:ignore U1000 Ignore unused function temporarily for debugging
func (b *AbelianIndexer) parseNullData(pkScript []byte) (string, error) {
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
func (b *AbelianIndexer) parseAddress(pkScript []byte) (string, error) {
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
func (b *AbelianIndexer) LatestBlock() (int64, error) {
	resp, err := b.getResponseFromChan("getblockcount", nil)
	if err != nil {
		return 0, err
	}
	number, err := strconv.ParseInt(string(resp), 0, 64)
	if err != nil {
		return 0, err
	}
	return number, nil
}

// BlockChainInfo get block chain info
func (b *AbelianIndexer) BlockChainInfo() (*types.BlockChainInfo, error) {
	resp, err := b.getResponseFromChan("getinfo", nil)
	if err != nil {
		return nil, err
	}

	var abe AbelianChainInfo
	err = json.Unmarshal(resp, &abe)
	if err != nil {
		return nil, err
	}

	blockchainInfo := &types.BlockChainInfo{
		Chain:  "0", //todo temp
		Blocks: abe.Blocks,
		Data:   abe,
	}

	return blockchainInfo, nil
}

func (b *AbelianIndexer) GetRawTransactionVerbose(hash string) (*types.TxInfo, error) {
	if has0xPrefix(hash) {
		hash = strings.Replace(hash, "0x", "", 1)
	}

	params := make([]interface{}, 0, 2)
	params = append(params, hash, true)
	resp, err := b.getResponseFromChan("getrawtransaction", params)
	if err != nil {
		return nil, err
	}

	var abeTx AbecTx
	err = json.Unmarshal(resp, &abeTx)
	if err != nil {
		return nil, err
	}

	tx := &types.TxInfo{
		Hash:          hash,
		Confirmations: abeTx.Confirmations,
		Data:          abeTx,
	}
	return tx, nil
}

func (b *AbelianIndexer) GetRawTransaction(txHash *chainhash.Hash) (*types.TxInfo, error) {
	return b.GetRawTransactionVerbose(txHash.String())
}

// GetBlockByHeight returns a raw block from the server given its height
func (b *AbelianIndexer) GetBlockByHeight(height int64) (*types.BlockInfo, error) {
	//blockhash, err := b.client.GetBlockHash(height)
	//if err != nil {
	//	return nil, err
	//}

	params := make([]interface{}, 0, 1)
	params = append(params, height)
	resp, err := b.getResponseFromChan("getblockhash", params)
	if err != nil {
		return nil, err
	}

	blockHash := string(resp)
	params2 := make([]interface{}, 0, 1)
	params2 = append(params2, blockHash, 2)
	resp, err = b.getResponseFromChan("getblockabe", params2)
	if err != nil {
		return nil, err
	}

	var abeBlock AbecBlock
	err = json.Unmarshal(resp, &abeBlock)
	if err != nil {
		return nil, err
	}

	block := &types.BlockInfo{
		Height:    height,
		BlockHash: abeBlock.BlockHash,
		Time:      abeBlock.Time,
		Data:      abeBlock,
	}

	return block, nil
}

type AbecBlock struct {
	Height        int64     `json:"height"`
	Confirmations int64     `json:"confirmations"`
	Version       int64     `json:"version"`
	VersionHex    string    `json:"versionHex"`
	Time          int64     `json:"time"`
	Nonce         uint64    `json:"nonce"`
	Size          int64     `json:"size"`
	FullSize      int64     `json:"fullsize"`
	Difficulty    float64   `json:"difficulty"`
	BlockHash     string    `json:"hash"`
	PrevBlockHash string    `json:"previousblockhash"`
	NextBlockHash string    `json:"nextblockhash"`
	ContentHash   string    `json:"contenthash"`
	MerkleRoot    string    `json:"merkleroot"`
	Bits          string    `json:"bits"`
	SealHash      string    `json:"sealhash"`
	Mixdigest     string    `json:"mixdigest"`
	TxHashes      []string  `json:"tx"`
	RawTxs        []*AbecTx `json:"rawTx"`
}

type AbecTx struct {
	Hex           string        `json:"hex"`
	TxID          string        `json:"txid"`
	TxHash        string        `json:"hash"`
	Time          int64         `json:"time"`
	BlockHash     string        `json:"blockhash"`
	BlockTime     int64         `json:"blocktime"`
	Confirmations uint64        `bson:"confirmations"`
	Version       int64         `json:"version"`
	Size          int64         `json:"size"`
	FullSize      int64         `json:"fullsize"`
	Fee           float64       `json:"fee"`
	Witness       string        `json:"witness"`
	Vin           []*AbecTxVin  `json:"vin"`
	Vout          []*AbecTxVout `json:"vout"`
}

type AbecTxVin struct {
	UTXORing     AbecUTXORing `json:"prevutxoring"`
	SerialNumber string       `json:"serialnumber"`
}

type AbecUTXORing struct {
	Version     int64    `json:"version"`
	BlockHashes []string `json:"blockhashs"`
	OutPoints   []struct {
		TxHash string `json:"txid"`
		Index  int64  `json:"index"`
	} `json:"outpoints"`
}

type AbecTxVout struct {
	N      int64  `json:"n"`
	Script string `json:"script"`
}

type AbecJSONRPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      string        `json:"id"`
}

type AbecJSONRPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
	ID     string          `json:"id"`
}

type AbelianChainInfo struct {
	Protocolversion      int     `json:"protocolversion" gorm:"column:protocolversion"`
	Relayfee             float64 `json:"relayfee" gorm:"column:relayfee"`
	Nodetype             string  `json:"nodetype" gorm:"column:nodetype"`
	Timeoffset           int64   `json:"timeoffset" gorm:"column:timeoffset"`
	Blocks               int64   `json:"blocks" gorm:"column:blocks"`
	Witnessserviceheight int64   `json:"witnessserviceheight" gorm:"column:witnessserviceheight"`
	Version              int64   `json:"version" gorm:"column:version"`
	Difficulty           float64 `json:"difficulty" gorm:"column:difficulty"`
	Proxy                string  `json:"proxy" gorm:"column:proxy"`
	Worksum              string  `json:"worksum" gorm:"column:worksum"`
	Bestblockhash        string  `json:"bestblockhash" gorm:"column:bestblockhash"`
	Testnet              bool    `json:"testnet" gorm:"column:testnet"`
	Connections          int64   `json:"connections" gorm:"column:connections"`
	Errors               string  `json:"errors" gorm:"column:errors"`
}
