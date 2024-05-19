package indexer

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/b2network/b2-indexer/internal/config"
	"github.com/b2network/b2-indexer/internal/model"
	"github.com/b2network/b2-indexer/internal/types"
	"github.com/b2network/b2-indexer/pkg/log"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/tidwall/gjson"
)

// AbelianIndexer bitcoin indexer, parse and forward data
type AbelianIndexer struct {
	client *rpcclient.Client // call bitcoin rpc client
	//chainParams         *chaincfg.Params  // bitcoin network params, e.g. mainnet, testnet, etc.
	listenAddress       string // need listened bitcoin address
	targetConfirmations uint64
	config              *config.BitcoinConfig
	ctx                 *model.Context
	logger              log.Logger
}

// NewAbelianIndexer new bitcoin indexer
func NewAbelianIndexer(log log.Logger, ctx *model.Context, listenAddress string, targetConfirmations uint64) (types.TxIndexer, error) {
	// check listenAddress
	//address, err := btcutil.DecodeAddress(listenAddress, chainParams)
	//if err != nil {
	//	return nil, fmt.Errorf("%w:%s", ErrDecodeListenAddress, err.Error())
	//}

	bitcoinCfg := ctx.BitcoinConfig
	return &AbelianIndexer{
		logger:              log,
		client:              nil,
		listenAddress:       listenAddress,
		ctx:                 ctx,
		config:              bitcoinCfg,
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
	url := bitcoinCfg.RPCHost

	if len(bitcoinCfg.RPCPort) > 1 {
		url = fmt.Sprintf("%s:%s", bitcoinCfg.RPCHost, bitcoinCfg.RPCPort)
	}

	if !strings.HasPrefix(url, "http") && !strings.HasPrefix(url, "https") {

		if bitcoinCfg.DisableTLS {
			url = fmt.Sprintf("https://%s", url)
		} else {
			url = fmt.Sprintf("http://%s", url)
		}
	}

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
	root := gjson.ParseBytes(body)
	respObj.Result = []byte(root.Get("result").String())
	respObj.Error = []byte(root.Get("error").String())
	respObj.ID = root.Get("id").String()

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

	if len(txResult.Memo) < 9 {
		b.logger.Warnf("tx has not fount memo, hash:%v", txResult.TxID)
		return nil, nil
	}
	bs, err := hex.DecodeString(txResult.Memo)
	if err != nil {
		return nil, fmt.Errorf("decode memo error:%w", err)
	}

	//procf := bs[:8]

	//test case
	//str := `
	//{
	//   "action": "deposit",
	//   "protocol": "Mable",
	//   "from": "0xCB369d06BD0aaA813E1d6bad09421D53bB96D175",
	//   "to": "0xE37e799D5077682FA0a244D46E5649F71457BD09",
	//   "receipt": "0x1111111254fb6c44bAC0beD2854e76F90643097d",
	//   "value": "0x10"
	//}
	//`

	/**
	{"action":"inscribe","protocol":"Mable","from":"abe32f5c9dd67b6f0e11333fc54e4b54d1f05456ea0e2abc6e1459b056271e3de6180f7cca4ca880a8839c72d412987ffd47d7fdca60fce5838bfcbea68dd741146b","networkname":"abe-test","proofRootHash":"2dba5dbc339e7316aea2683faf839c1b7b1ee2313db792112588118df066aa35","stateRootHash":"088314330cd2c7929b88219179fe0c69c5fd85176a1c5b0f7de56591e283e45c"}
	*/
	//b.listenAddress = "0xE37e799D5077682FA0a244D46E5649F71457BD09"

	//memo := []byte(str)

	if len(bs) < 9 {
		return nil, fmt.Errorf("decode memo error:%w", err)
	}

	memo := bs[8:]
	if len(memo) < 1 {
		return nil, fmt.Errorf("parse memo error, len:%d, memo:%v", len(memo), string(memo))
	}

	action := gjson.ParseBytes(memo).Get("action").String()
	protocol := gjson.ParseBytes(memo).Get("protocol").String()

	if action != "deposit" && protocol != "Mable" {
		return nil, nil
	}

	var m Memo
	err = json.Unmarshal(memo, &m)
	if err != nil {
		return nil, fmt.Errorf("unmarshal memo error, memo:%v", string(memo))
	}

	listenAddress := b.listenAddress
	if has0xPrefix(listenAddress) {
		listenAddress = strings.Replace(listenAddress, "0x", "", 1)
	}
	toAddress := m.To
	if has0xPrefix(toAddress) {
		toAddress = strings.Replace(toAddress, "0x", "", 1)
	}

	fromAddress := m.From
	if has0xPrefix(fromAddress) {
		fromAddress = strings.Replace(fromAddress, "0x", "", 1)
	}

	hasListenAddress := false
	if listenAddress == toAddress {
		hasListenAddress = true
	}
	totalValue, _ := strconv.ParseInt(m.Value, 0, 64)

	tos := make([]types.BitcoinTo, 0)
	parseTo := types.BitcoinTo{
		Address: m.Receipt,
		Value:   totalValue,
	}
	tos = append(tos, parseTo)

	//tos, totalValue, listenAddress, _ := b.parseToAddress(txResult.Vout)
	//to is listenAddress and from is not fromAddress
	if hasListenAddress && listenAddress != fromAddress {
		//fromAddress, err := b.parseFromAddress(txResult.Vin)
		//if err != nil {
		//	return nil, fmt.Errorf("vin parse err:%w", err)
		//}

		// TODO: temp fix, if from is listened address, continue
		if len(m.From) == 0 {
			b.logger.Warnw("parse from address empty or nonsupport tx type",
				"txId", txResult.TxID,
				"listenAddress", b.listenAddress)
			return nil, nil
		}

		return &types.BitcoinTxParseResult{
			TxID:   txResult.TxID,
			TxType: TxTypeTransfer,
			Index:  int64(index),
			Value:  totalValue,
			From: []types.BitcoinFrom{types.BitcoinFrom{
				Address: m.From,
			}},
			To:  b.listenAddress,
			Tos: tos,
		}, nil
	}
	return nil, nil
}

// parseAddress from pkscript parse address
//func (b *AbelianIndexer) ParseAddress(pkScript []byte) (string, error) {
//	return b.parseAddress(pkScript)
//}

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
		Chain:  strconv.FormatInt(abe.NetId, 10),
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

	//blockHash := string(resp)
	params2 := make([]interface{}, 0, 1)
	params2 = append(params2, string(resp), 2)
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
	Memo          string        `json:"memo"`
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
	Result []byte `json:"result"`
	Error  []byte `json:"error"`
	ID     string `json:"id"`
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
	NetId                int64   `json:"netid" gorm:"column:netid"`
}

/**
{
 "action":"deposit",//操作 Required
 "protocol":"Mable", //协议 Required
 "from":"0xCB369d06BD0aaA813E1d6bad09421D53bB96D175",//交易发送者 Required
 "to":"0xE37e799D5077682FA0a244D46E5649F71457BD09",// 交易到达者 Optional,default address is specified by the operator
 "receipt":"0x1111111254fb6c44bAC0beD2854e76F90643097d",// L2 接收者 Optional, the value is from memo or bridge portal
 "value":"0x10" //存款金额 Required
}
*/

type Memo struct {
	Action   string `json:"action"`
	Protocol string `json:"protocol"`
	From     string `json:"from"`
	To       string `json:"to"`
	Value    string `json:"value"`
	Receipt  string `json:"receipt"`
}
