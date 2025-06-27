package indexer

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/qday-io/qday-abel-bridge-indexer/config"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/cometbft/cometbft/libs/service"
	"github.com/qday-io/qday-abel-bridge-indexer/internal/model"
	"github.com/qday-io/qday-abel-bridge-indexer/pkg/log"
	"gorm.io/gorm"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	BridgeWithdrawServiceName = "BitcoinBridgeWithdrawService"
	WithdrawHandleTime        = 10
	WithdrawTXConfirmTime     = 60 * 5

	// P2SHSize 23 bytes.
	P2SHSize = 23
	// P2SHOutputSize 32 bytes
	//      - value: 8 bytes
	//      - var_int: 1 byte (pkscript_length)
	//      - pkscript (p2sh): 23 bytes
	P2SHOutputSize = 8 + 1 + P2SHSize
	// InputSize 41 bytes
	//	- PreviousOutPoint:
	//		- Hash: 32 bytes
	//		- Index: 4 bytes
	//	- OP_DATA: 1 byte (ScriptSigLength)
	//	- ScriptSig: 0 bytes
	//	- Witness <----	we use "Witness" instead of "ScriptSig" for
	// 			transaction validation, but "Witness" is stored
	// 			separately and weight for it size is smaller. So
	// 			we separate the calculation of ordinary data
	// 			from witness data.
	//	- Sequence: 4 bytes
	InputSize = 32 + 4 + 1 + 4
	// MultiSigSize 71 bytes
	//	- OP_2: 1 byte
	//	- OP_DATA: 1 byte (pubKeyAlice length)
	//	- pubKeyAlice: 33 bytes
	//	- OP_DATA: 1 byte (pubKeyBob length)
	//	- pubKeyBob: 33 bytes
	//	- OP_2: 1 byte
	//	- OP_CHECKMULTISIG: 1 byte
	MultiSigSize = 1 + 1 + 33 + 1 + 33 + 1 + 1
)

// BridgeWithdrawService indexes transactions for json-rpc service.
type BridgeWithdrawService struct {
	service.BaseService

	btcCli *rpcclient.Client
	ethCli *ethclient.Client
	config *config.BitcoinConfig
	db     *gorm.DB
	log    log.Logger
}

// NewBridgeWithdrawService returns a new service instance.
func NewBridgeWithdrawService(
	btcCli *rpcclient.Client,
	ethCli *ethclient.Client,
	config *config.BitcoinConfig,
	db *gorm.DB,
	log log.Logger,
) *BridgeWithdrawService {
	is := &BridgeWithdrawService{btcCli: btcCli, ethCli: ethCli, config: config, db: db, log: log}
	is.BaseService = *service.NewBaseService(nil, BridgeWithdrawServiceName, is)
	return is
}

// OnStart implements service.Service by subscribing for new blocks
// and indexing them by events.
func (bis *BridgeWithdrawService) OnStart() error {
	if !bis.db.Migrator().HasTable(&model.Withdraw{}) {
		err := bis.db.AutoMigrate(&model.Withdraw{})
		if err != nil {
			bis.log.Errorw("BridgeWithdrawService create withdraw table", "error", err.Error())
			return err
		}
	}
	if !bis.db.Migrator().HasTable(&model.WithdrawTx{}) {
		err := bis.db.AutoMigrate(&model.WithdrawTx{})
		if err != nil {
			bis.log.Errorw("BridgeWithdrawService create withdrawTx table", "error", err.Error())
			return err
		}
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				bis.log.Errorw("BridgeWithdrawService panic", "error", r)
			}
		}()
		for {
			// broadcast transaction
			time.Sleep(time.Duration(WithdrawHandleTime) * time.Second)
			var withdrawTxList []model.WithdrawTx
			err := bis.db.Model(&model.WithdrawTx{}).Where(fmt.Sprintf("%s = ?", model.Withdraw{}.Column().Status), model.BtcTxWithdrawSignatureCompleted).Find(&withdrawTxList).Error
			if err != nil {
				bis.log.Errorw("BridgeWithdrawService get broadcast tx failed", "error", err)
				continue
			}
			if len(withdrawTxList) == 0 {
				continue
			}
			for _, v := range withdrawTxList {
				pack, err := psbt.NewFromRawBytes(strings.NewReader(v.BtcTx), true)
				if err != nil {
					bis.log.Errorw("BridgeWithdrawService get psbt tx err", "error", err)
					continue
				}
				tx := pack.UnsignedTx
				preTx := pack.Inputs

				var signes [][]model.Sign
				var signatures []string
				for _, signHex := range signatures {
					signByte, err := hex.DecodeString(signHex)
					if err != nil {
						bis.log.Errorw("BridgeWithdrawService DecodeString sign err", "error", err)
						continue
					}
					var sign []model.Sign
					err = json.Unmarshal(signByte, &sign)
					if err != nil {
						bis.log.Errorw("BridgeWithdrawService Unmarshal sign err", "error", err)
						continue
					}
					signes = append(signes, sign)
				}
				for index, in := range tx.TxIn {
					witness := wire.TxWitness{nil}
					for i := 0; i < bis.config.Bridge.MultisigNum; i++ {
						sign := signes[i][index].Sign
						witness = append(witness, sign)
					}
					witness = append(witness, preTx[index].WitnessUtxo.PkScript)
					in.Witness = witness
				}
				var status int
				var reason string
				txHash, err := bis.btcCli.SendRawTransaction(tx, true)
				if err != nil {
					bis.log.Errorw("BridgeWithdrawService broadcast tx err", "id", v.ID, "txID", v.BtcTxID, "error", err)
					status = model.BtcTxWithdrawBroadcastFailed
					reason = err.Error()
				} else {
					status = model.BtcTxWithdrawBroadcastSuccess
				}
				updateFields := map[string]interface{}{
					model.WithdrawTx{}.Column().BtcTxHash: txHash,
					model.WithdrawTx{}.Column().Status:    status,
					model.WithdrawTx{}.Column().Reason:    reason,
				}
				err = bis.db.Model(&model.WithdrawTx{}).Where("id = ?", v.ID).Updates(updateFields).Error
				if err != nil {
					bis.log.Errorw("BridgeWithdrawService broadcast tx update db err", "error", err, "id", v.ID)
					continue
				}
				bis.log.Infow("BridgeWithdrawService broadcast tx success", "id", v.ID, "btcTxID", v.BtcTxID)
			}
		}
	}()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				bis.log.Errorw("BridgeWithdrawService panic", "error", r)
			}
		}()
		for {
			time.Sleep(time.Duration(WithdrawTXConfirmTime) * time.Second)
			// confirm tx
			var withdrawTxList []model.WithdrawTx
			err := bis.db.Model(&model.WithdrawTx{}).Where(fmt.Sprintf("%s = ?", model.WithdrawTx{}.Column().Status), model.BtcTxWithdrawBroadcastSuccess).Find(&withdrawTxList).Error
			if err != nil {
				bis.log.Errorw("BridgeWithdrawService get broadcast tx failed", "error", err)
				continue
			}
			if len(withdrawTxList) == 0 {
				continue
			}
			for _, v := range withdrawTxList {
				txHash, err := chainhash.NewHashFromStr(v.BtcTxHash)
				if err != nil {
					bis.log.Errorw("BridgeWithdrawService NewHashFromStr err", "error", err, "txhash", v.BtcTxHash)
					continue
				}
				txRawResult, err := bis.btcCli.GetRawTransactionVerbose(txHash)
				if err != nil {
					bis.log.Errorw("BridgeWithdrawService GetRawTransactionVerbose err", "error", err, "txID", v.BtcTxID)
					continue
				}
				if txRawResult.Confirmations >= 6 {
					err = bis.db.Model(&model.WithdrawTx{}).Where("id = ?", v.ID).Update(model.WithdrawTx{}.Column().Status, model.BtcTxWithdrawConfirmed).Error
					if err != nil {
						bis.log.Errorw("BridgeWithdrawService Update WithdrawTx status err", "error", err, "txID", v.BtcTxID)
						continue
					}
				}
			}
		}
	}()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				bis.log.Errorw("BridgeWithdrawService panic", "error", r)
			}
		}()
		for {
			time.Sleep(time.Duration(WithdrawHandleTime) * time.Second)
			// complete tx
			var withdrawTxList []model.WithdrawTx
			err := bis.db.Model(&model.WithdrawTx{}).
				Where(fmt.Sprintf("%s = ? OR %s = ?", model.WithdrawTx{}.Column().Status, model.WithdrawTx{}.Column().Status), model.BtcTxWithdrawConfirmed, model.BtcTxWithdrawBroadcastFailed).
				Find(&withdrawTxList).Error
			if err != nil {
				bis.log.Errorw("BridgeWithdrawService get broadcast tx failed", "error", err)
				continue
			}
			if len(withdrawTxList) == 0 {
				continue
			}
			for _, v := range withdrawTxList {
				var withdrawTxStatus int
				var withdrawHistoryStatus int
				if v.Status == model.BtcTxWithdrawConfirmed {
					withdrawTxStatus = model.BtcTxWithdrawSuccess
					withdrawHistoryStatus = model.BtcTxWithdrawSuccess
				} else {
					withdrawTxStatus = model.BtcTxWithdrawFailed
					withdrawHistoryStatus = model.BtcTxWithdrawPending
				}
				err = bis.db.Transaction(func(tx *gorm.DB) error {
					err = tx.Model(&model.WithdrawTx{}).Where("id = ?", v.ID).Update(model.WithdrawTx{}.Column().Status, withdrawTxStatus).Error
					if err != nil {
						bis.log.Errorw("BridgeWithdrawService Update WithdrawTx status err", "error", err, "txID", v.BtcTxID)
						return err
					}
					var b2TxHashList []string
					err = json.Unmarshal([]byte(v.B2TxHashes), &b2TxHashList)
					if err != nil {
						return err
					}
					err = tx.Model(&model.Withdraw{}).Where(fmt.Sprintf("%s in (?)", model.Withdraw{}.Column().B2TxHash), b2TxHashList).Update(model.Withdraw{}.Column().Status, withdrawHistoryStatus).Error
					if err != nil {
						bis.log.Errorw("BridgeWithdrawService Update WithdrawTx status err", "error", err, "txID", v.BtcTxID)
						return err
					}
					return nil
				})
				if err != nil {
					bis.log.Errorw("BridgeWithdrawService complete WithdrawTx err", "error", err, "txID", v.BtcTxID)
				}
			}
		}
	}()

	for {
		timeInterval := bis.config.Bridge.TimeInterval
		time.Sleep(time.Duration(timeInterval) * time.Second)
		var withdrawList []model.Withdraw
		err := bis.db.Model(&model.Withdraw{}).Where(fmt.Sprintf("%s = ?", model.Withdraw{}.Column().Status), model.BtcTxWithdrawPending).Find(&withdrawList).Error
		if err != nil {
			bis.log.Errorw("BridgeWithdrawService get blockNumber failed", "error", err)
			continue
		}
		if len(withdrawList) == 0 {
			continue
		}
		var destAddressList []string
		var amounts []int64
		var ids []int64
		var b2TxHashes []string
		for _, v := range withdrawList {
			ids = append(ids, v.ID)
			destAddressList = append(destAddressList, v.BtcTo)
			amounts = append(amounts, v.BtcValue)
			b2TxHashes = append(b2TxHashes, v.B2TxHash)
		}
		if len(ids) == 0 {
			continue
		}
		b2TxHashesByte, err := json.Marshal(b2TxHashes)
		if err != nil {
			bis.log.Errorw("BridgeWithdrawService Marshal b2TxHashes err", "error", err, "id", ids)
			continue
		}
		txID, btcTx, err := bis.ConstructTx(destAddressList, amounts, b2TxHashesByte)
		if err != nil {
			if errors.Is(err, errors.New("no unspent tx")) {
				continue
			}
			bis.log.Errorw("BridgeWithdrawService transferToBtc failed: ", "error", err)
			continue
		}
		err = bis.db.Transaction(func(tx *gorm.DB) error {
			err = tx.Model(&model.Withdraw{}).Where("id in (?)", ids).Update(model.Withdraw{}.Column().Status, model.BtcTxWithdrawSubmitTxMsg).Error
			if err != nil {
				bis.log.Errorw("BridgeWithdrawService submit withdraw tx update db err", "error", err, "id", ids)
				return err
			}
			withdrawTxData := model.WithdrawTx{
				BtcTxID:    txID,
				BtcTx:      btcTx,
				B2TxHashes: string(b2TxHashesByte),
			}
			var withdrawTx model.WithdrawTx
			result := tx.Model(&model.WithdrawTx{}).Where(fmt.Sprintf("%s = ?", model.WithdrawTx{}.Column().BtcTxID), txID).First(&withdrawTx)
			if result.RowsAffected == 0 {
				if err = tx.Create(&withdrawTxData).Error; err != nil {
					bis.log.Errorw("BridgeWithdrawService create withdrawTx err", "b2TxHashes", b2TxHashes, "error", err)
					return err
				}
			} else {
				updateFields := map[string]interface{}{
					model.WithdrawTx{}.Column().Status: model.BtcTxWithdrawPending,
					model.WithdrawTx{}.Column().Reason: "",
				}
				err = tx.Model(&model.WithdrawTx{}).Where("id = ?", withdrawTx.ID).Updates(updateFields).Error
				if err != nil {
					bis.log.Errorw("BridgeWithdrawService Update WithdrawTx status err", "error", err, "txID", withdrawTx.BtcTxID)
					return err
				}
			}

			bis.log.Infow("BridgeWithdrawService broadcast tx success", "id", ids, "b2TxHashes", b2TxHashes)
			return nil
		})
		if err != nil {
			bis.log.Errorw("BridgeWithdrawService submit withdrawTx err", "error", err, "txID", txID)
		}
	}
}

func (bis *BridgeWithdrawService) BroadcastTx(tx *wire.MsgTx) (*chainhash.Hash, error) {
	mempoolURL := bis.GetMempoolURL()
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/tx", mempoolURL), strings.NewReader(hex.EncodeToString(buf.Bytes())))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	txHash, err := chainhash.NewHashFromStr(string(body))
	if err != nil {
		return nil, err
	}
	return txHash, nil
}

func (bis *BridgeWithdrawService) GetUnspentList(address string, cursor int64) (int64, int64, []*model.UnspentOutput, error) {
	var total int64
	var satoshiTotal int64
	url := bis.GetUisatURL()
	client := resty.New()
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", bis.config.Bridge.UnisatAPIKey)).
		Get(fmt.Sprintf("%s/v1/indexer/address/%s/utxo-data?cursor=%d", url, address, cursor))
	if err != nil {
		bis.log.Errorw("BridgeWithdrawService client url err", "error", err)
		return total, satoshiTotal, nil, err
	}
	if resp.StatusCode() != 200 {
		bis.log.Errorw("BridgeWithdrawService client res err", "res", resp)
		return total, satoshiTotal, nil, errors.New(resp.Status())
	}
	var respData model.UnisatResponse
	err = json.Unmarshal(resp.Body(), &respData)
	if err != nil {
		bis.log.Errorw("BridgeWithdrawService unmarshal err", "error", err)
		return total, satoshiTotal, nil, err
	}
	if respData.Code != 0 {
		bis.log.Errorw("BridgeWithdrawService get utxo data err", "resp", respData)
		return total, satoshiTotal, nil, errors.New(respData.Msg)
	}
	utxoDataByte, err := json.Marshal(respData.Data)
	if err != nil {
		bis.log.Errorw("BridgeWithdrawService marshal utxo data err", "error", err)
		return total, satoshiTotal, nil, err
	}
	var utxoData model.UtxoData
	err = json.Unmarshal(utxoDataByte, &utxoData)
	if err != nil {
		bis.log.Errorw("BridgeWithdrawService unmarshal utxo data err", "error", err)
		return total, satoshiTotal, nil, err
	}
	unspentOutputs := make([]*model.UnspentOutput, 0)
	total = utxoData.Total
	for _, v := range utxoData.Utxo {
		txHash, err := chainhash.NewHashFromStr(v.Txid)
		if err != nil {
			return total, satoshiTotal, nil, err
		}
		unspentOutputs = append(unspentOutputs, &model.UnspentOutput{
			Outpoint: wire.NewOutPoint(txHash, uint32(v.Vout)),
			Output:   wire.NewTxOut(v.Satoshi, []byte(v.ScriptPk)),
		})
		satoshiTotal += v.Satoshi
	}
	return total, satoshiTotal, unspentOutputs, nil
}

func (bis *BridgeWithdrawService) GetUisatURL() string {
	networkName := bis.config.NetworkName
	switch networkName {
	case chaincfg.MainNetParams.Name:
		return "https://open-api.unisat.io"
	case chaincfg.TestNet3Params.Name, "testnet":
		return "https://open-api-testnet.unisat.io"
	}
	return ""
}

func (bis *BridgeWithdrawService) GetMempoolURL() string {
	networkName := bis.config.NetworkName
	switch networkName {
	case chaincfg.MainNetParams.Name:
		return "https://mempool.space/api"
	case chaincfg.TestNet3Params.Name, "testnet":
		return "https://mempool.space/testnet/api"
	case chaincfg.SigNetParams.Name:
		return "https://mempool.space/signet/api"
	}
	return ""
}

func (bis *BridgeWithdrawService) ConstructTx(destAddressList []string, amounts []int64, b2TxHashes []byte) (string, string, error) {
	sourceAddrStr := bis.config.IndexerListenAddress
	destAddressList, amounts = mergeDuplicateAddresses(destAddressList, amounts)

	var defaultNet *chaincfg.Params
	networkName := bis.config.NetworkName
	defaultNet = config.ChainParams(networkName)

	// get sourceAddress UTXO
	sourceAddr, err := btcutil.DecodeAddress(sourceAddrStr, defaultNet)
	if err != nil {
		bis.log.Errorw("BridgeWithdrawService ConstructTx DecodeAddress err: ", "error", err)
		return "", "", err
	}

	total, satoshiTotal, unspentTxs, err := bis.GetUnspentList(sourceAddrStr, 0)
	if err != nil {
		bis.log.Errorw("BridgeWithdrawService GetUnspentList err: ", "error", err)
		return "", "", err
	}
	if len(unspentTxs) == 0 {
		return "", "", errors.New("no unspent tx")
	}
	var totalTransferAmount int64
	for _, v := range amounts {
		totalTransferAmount += v
	}
	if satoshiTotal <= totalTransferAmount {
		for i := 0; int64(i) < total/16; i++ {
			_, satoshiTotalTemp, unspentTxsTemp, err := bis.GetUnspentList(sourceAddrStr, int64(i))
			if err != nil {
				bis.log.Errorw("BridgeWithdrawService GetUnspentList err: ", "error", err)
				return "", "", err
			}
			if (satoshiTotal + satoshiTotalTemp) > totalTransferAmount {
				break
			}
			unspentTxs = append(unspentTxs, unspentTxsTemp...)
		}
	}

	tx := wire.NewMsgTx(wire.TxVersion)
	changeScript, err := txscript.PayToAddrScript(sourceAddr)
	if err != nil {
		bis.log.Errorw("BridgeWithdrawService transferToBtc PayToAddrScript sourceAddr failed: ", "error", err)
		return "", "", err
	}
	var txSize int
	var outputSize int
	var fee int
	for index, destAddress := range destAddressList {
		destAddr, err := btcutil.DecodeAddress(destAddress, defaultNet)
		if err != nil {
			bis.log.Errorw("BridgeWithdrawService transferToBtc DecodeAddress destAddress failed: ", "error", err)
			return "", "", err
		}
		destinationScript, err := txscript.PayToAddrScript(destAddr)
		if err != nil {
			bis.log.Errorw("BridgeWithdrawService transferToBtc PayToAddrScript destAddress failed: ", "error", err)
			return "", "", err
		}
		tx.AddTxOut(wire.NewTxOut(amounts[index], destinationScript))
		outputSize += wire.NewTxOut(amounts[index], destinationScript).SerializeSize()
	}
	outputSize += wire.NewTxOut(0, changeScript).SerializeSize()
	var pInput psbt.PInput
	feeRate, err := bis.GetFeeRate()
	if err != nil {
		bis.log.Errorw("BridgeWithdrawService GetFeeRate err: ", "error", err)
		return "", "", err
	}
	txSize += outputSize
	pInputArry := make([]psbt.PInput, 0)
	totalInputAmount := btcutil.Amount(0)
	for _, unspentTx := range unspentTxs {
		var inputSize int
		outpoint := wire.NewOutPoint(&unspentTx.Outpoint.Hash, unspentTx.Outpoint.Index)
		txIn := wire.NewTxIn(outpoint, nil, nil)
		tx.AddTxIn(txIn)
		multiSigScript, err := bis.GetMultiSigScript(bis.config.Bridge.PublicKeys, bis.config.Bridge.MultisigNum)
		if err != nil {
			bis.log.Errorw("BridgeWithdrawService ConstructTx GenerateMultiSigScript err", "error", err)
			return "", "", err
		}
		unspentTx.Output.PkScript = multiSigScript
		pInput.WitnessUtxo = unspentTx.Output
		pInput.WitnessScript = multiSigScript
		pInputArry = append(pInputArry, pInput)
		totalInputAmount += btcutil.Amount(unspentTx.Output.Value)
		inputSize = InputSize + bis.GetMultiSigWitnessSize()
		txSize += inputSize
		fee = txSize * feeRate.FastestFee
		if int64(totalInputAmount) > (totalTransferAmount + int64(fee)) {
			break
		}
	}
	changeAmount := int64(totalInputAmount) - int64(fee) - totalTransferAmount
	if changeAmount < 0 {
		bis.log.Errorw("BridgeWithdrawService ConstructTx insufficient balance err",
			"totalInputAmount", totalInputAmount, "fee", fee, "totalTransferAmount", totalTransferAmount)
		return "", "", errors.New("insufficient balance")
	}
	tx.AddTxOut(wire.NewTxOut(changeAmount, changeScript))
	bis.log.Infow("BridgeWithdrawService ConstructTx fee", "tx_id", tx.TxHash().String(), "fee", fee, "feeRate", feeRate)

	txCopy := tx.Copy()
	unsignedPsbt, err := psbt.NewFromUnsignedTx(txCopy)
	if err != nil {
		bis.log.Errorw("BridgeWithdrawService NewFromUnsignedTx err: ", "error", err)
		return "", "", err
	}
	unsignedPsbt.Inputs = pInputArry
	var unknown psbt.Unknown
	var unknowns []*psbt.Unknown
	unknown.Key = []byte("b2TxHashes")
	unknown.Value = b2TxHashes
	unknowns = append(unknowns, &unknown)
	unsignedPsbt.Unknowns = unknowns
	psbtData, err := unsignedPsbt.B64Encode()
	if err != nil {
		return "", "", err
	}
	return tx.TxHash().String(), psbtData, nil
}

func (bis *BridgeWithdrawService) GetMultiSigScript(pubs []string, minSignNum int) ([]byte, error) {
	var defaultNet *chaincfg.Params
	networkName := bis.config.NetworkName
	defaultNet = config.ChainParams(networkName)

	addressPubKeyList := make([]*btcutil.AddressPubKey, 0)
	for _, pubKey := range pubs {
		privateKeyByte01, err := hex.DecodeString(pubKey)
		if err != nil {
			bis.log.Errorw("BridgeWithdrawService GetMultiSigScript DecodeString err", "error", err)
			return nil, err
		}
		addressPubKey, err := btcutil.NewAddressPubKey(privateKeyByte01, defaultNet)
		if err != nil {
			bis.log.Errorw("BridgeWithdrawService GetMultiSigScript NewAddressPubKey err", "error", err)
			return nil, err
		}
		addressPubKeyList = append(addressPubKeyList, addressPubKey)
	}

	multiSigScript, err := txscript.MultiSigScript(addressPubKeyList, minSignNum)
	if err != nil {
		bis.log.Errorw("BridgeWithdrawService get MultiSigScript err", "error", err)
		return nil, err
	}
	return multiSigScript, nil
}

func (bis *BridgeWithdrawService) GetFeeRate() (*model.FeeRates, error) {
	mempoolURL := bis.GetMempoolURL()
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/fees/recommended", mempoolURL), strings.NewReader(""))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var feeRates model.FeeRates
	err = json.Unmarshal(body, &feeRates)
	if err != nil {
		return nil, err
	}
	return &feeRates, nil
}

func (bis *BridgeWithdrawService) GetMultiSigWitnessSize() int {
	//	- NumberOfWitnessElements: 1 byte
	//	- NilLength: 1 byte
	//	- sigAliceLength: 1 byte
	//	- sigAlice: 73 bytes
	//	- sigBobLength: 1 byte
	//	- sigBob: 73 bytes
	//	- WitnessScriptLength: 1 byte
	//	- WitnessScript (MultiSig)
	// MultiSigWitnessSize = 1 + 1 + 1 + 73 + 1 + 73 + 1 + MultiSigSize
	return 1 + 1 + 1 + MultiSigSize + bis.config.Bridge.MultisigNum*74
}

func mergeDuplicateAddresses(destAddressList []string, amounts []int64) ([]string, []int64) {
	mergedAddresses := make(map[string]int64)

	for i, address := range destAddressList {
		mergedAddresses[address] += amounts[i]
	}

	uniqueAddresses := make([]string, 0)
	mergedAmounts := make([]int64, 0)

	for address, amount := range mergedAddresses {
		uniqueAddresses = append(uniqueAddresses, address)
		mergedAmounts = append(mergedAmounts, amount)
	}

	return uniqueAddresses, mergedAmounts
}
