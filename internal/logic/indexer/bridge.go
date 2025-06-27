package indexer

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	config2 "github.com/qday-io/qday-abel-bridge-indexer/config"
	b2types "github.com/qday-io/qday-abel-bridge-indexer/internal/model"
	"github.com/qday-io/qday-abel-bridge-indexer/pkg/aa"
	"github.com/qday-io/qday-abel-bridge-indexer/pkg/log"
	"github.com/tidwall/gjson"
)

var (
	ErrBridgeDepositTxHashExist                 = errors.New("non-repeatable processing")
	ErrBridgeDepositContractInsufficientBalance = errors.New("insufficient balance")
	ErrBridgeWaitMinedStatus                    = errors.New("tx wait mined status failed")
	ErrBridgeFromGasInsufficient                = errors.New("gas required exceeds allowanc")
	ErrAAAddressNotFound                        = errors.New("address not found")
	ErrOldNonceToHeight                         = errors.New("old nonce params to height")
)

// Bridge bridge
// TODO: only L1 -> L2, More calls may be supported later
type Bridge struct {
	EthRPCURL            string
	EthPrivKey           *ecdsa.PrivateKey
	ContractAddress      common.Address
	ABI                  string
	BaseGasPriceMultiple int64
	B2ExplorerURL        string
	logger               log.Logger
	network              string
	// eoa transfer switch
	//enableEoaTransfer bool
	// aa server
	AAPubKeyAPI string
}
type B2ExplorerStatus struct {
	GasPrices struct {
		Fast    float64 `json:"fast"`
		Slow    float64 `json:"slow"`
		Average float64 `json:"average"`
	} `json:"gas_prices"`
}

var txLock sync.Mutex

// NewBridge new bridge
func NewBridge(bridgeCfg config2.BridgeConfig, abiFileDir string, log log.Logger, network string) (*Bridge, error) {
	rpcURL, err := url.ParseRequestURI(bridgeCfg.EthRPCURL)
	if err != nil {
		return nil, err
	}

	var ABI string

	// 优先使用配置中的 ABI，如果为空则使用默认 ABI
	if bridgeCfg.ABI != "" {
		ABI = bridgeCfg.ABI
	} else {
		// 尝试从文件读取 ABI（向后兼容）
		abiFile, err := os.ReadFile(path.Join(abiFileDir, "abi.json"))
		if err != nil {
			// 使用默认 ABI
			ABI = config2.DefaultDepositAbi
		} else {
			ABI = string(abiFile)
		}
	}

	//newParticle, err := particle.NewParticle(
	//	bridgeCfg.AAParticleRPC,
	//	bridgeCfg.AAParticleProjectID,
	//	bridgeCfg.AAParticleServerKey,
	//	bridgeCfg.AAParticleChainID)
	//if err != nil {
	//	return nil, err
	//}
	ethPrivKey := bridgeCfg.EthPrivKey

	//todo temp
	//bridgeCfg.EnableVSM = false

	if has0xPrefix(ethPrivKey) {
		ethPrivKey = ethPrivKey[2:]
	}
	privateKey, err := crypto.HexToECDSA(ethPrivKey)
	if err != nil {
		return nil, err
	}
	log.Infof("load eth address: %s", crypto.PubkeyToAddress(privateKey.PublicKey))
	return &Bridge{
		EthRPCURL:       rpcURL.String(),
		ContractAddress: common.HexToAddress(bridgeCfg.ContractAddress),
		EthPrivKey:      privateKey,
		ABI:             ABI,
		logger:          log,
		network:         network,
		//enableEoaTransfer:    bridgeCfg.EnableEoaTransfer,
		AAPubKeyAPI:          bridgeCfg.AAB2PI,
		BaseGasPriceMultiple: bridgeCfg.GasPriceMultiple,
		B2ExplorerURL:        bridgeCfg.B2ExplorerURL,
	}, nil
}

// Deposit to ethereum
func (b *Bridge) Deposit(
	hash string,
	bitcoinAddress b2types.BitcoinFrom,
	tos string,
	amount int64,
	oldTx *types.Transaction,
	nonce uint64,
	resetNonce bool,
) (*types.Transaction, []byte, string, string, error) {
	if bitcoinAddress.Address == "" {
		return nil, nil, "", "", fmt.Errorf("bitcoin address is empty")
	}

	if hash == "" {
		return nil, nil, "", "", fmt.Errorf("tx id is empty")
	}

	ctx := context.Background()

	toAddress, err := b.BitcoinAddressToEthAddress(hash, bitcoinAddress)
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("btc address to eth address err:%w", err)
	}

	//todo test case
	//toAddress := "0xdac17f958d2ee523a2206206994597c13d831ec7"

	list := gjson.Parse(tos).Array()
	var lockupPeriod uint64
	if len(list) > 0 {
		lockupPeriod = list[0].Get("Memo.lockupPeriod").Uint()
		//rewardRatio = list[0].Get("Memo.rewardRatio").Uint()
	}

	mintNum := new(big.Int).SetInt64(amount)
	powerOfTen := new(big.Int).SetInt64(1e11)
	mintNum = new(big.Int).Mul(mintNum, powerOfTen)

	//log.Infof("amount:%v,mint:%v,rant:%v", amount, mintNum.String(), powerOfTen.String())

	data, err := b.ABIPack(b.ABI, "mintWAbel", common.HexToAddress(toAddress), mintNum, new(big.Int).SetUint64(lockupPeriod))
	if err != nil {
		return nil, nil, toAddress, "", fmt.Errorf("abi pack err:%w", err)
	}

	if oldTx != nil {
		tx, err := b.retrySendTransaction(ctx, oldTx, b.EthPrivKey, resetNonce)
		if err != nil {
			return nil, nil, toAddress, "", err
		}
		return tx, oldTx.Data(), toAddress, b.FromAddress(), nil
	}

	tx, err := b.sendTransaction(ctx, b.EthPrivKey, b.ContractAddress, data, new(big.Int).SetInt64(0), nonce, resetNonce)
	if err != nil {
		return nil, nil, toAddress, "", err
	}

	b.logger.Infof("deposit success: hash:%v", tx.Hash().String())

	return tx, data, toAddress, b.FromAddress(), nil
}

// Transfer to ethereum
// TODO: temp handle, future remove
func (b *Bridge) Transfer(bitcoinAddress b2types.BitcoinFrom,
	amount int64,
	oldTx *types.Transaction,
	nonce uint64,
	resetNonce bool,
) (*types.Transaction, string, error) {
	if bitcoinAddress.Address == "" {
		return nil, "", fmt.Errorf("bitcoin address is empty")
	}

	ctx := context.Background()

	toAddress, err := b.BitcoinAddressToEthAddress("", bitcoinAddress)
	if err != nil {
		return nil, "", fmt.Errorf("btc address to eth address err:%w", err)
	}

	if oldTx != nil {
		receipt, err := b.retrySendTransaction(ctx,
			oldTx,
			b.EthPrivKey,
			resetNonce,
		)
		if err != nil {
			return nil, "", err
		}

		return receipt, b.FromAddress(), nil
	}

	receipt, err := b.sendTransaction(ctx,
		b.EthPrivKey,
		common.HexToAddress(toAddress),
		nil,
		new(big.Int).Mul(new(big.Int).SetInt64(amount), new(big.Int).SetInt64(10000000000)),
		nonce,
		resetNonce,
	)
	if err != nil {
		return nil, "", fmt.Errorf("eth call err:%w", err)
	}

	return receipt, b.FromAddress(), nil
}

func (b *Bridge) sendTransaction(ctx context.Context, fromPriv *ecdsa.PrivateKey,
	toAddress common.Address, data []byte, value *big.Int, oldNonce uint64, resetNonce bool,
) (*types.Transaction, error) {
	txLock.Lock()
	defer txLock.Unlock()
	client, err := ethclient.Dial(b.EthRPCURL)
	if err != nil {
		return nil, err
	}
	fromAddress := crypto.PubkeyToAddress(fromPriv.PublicKey)

	nonce, err := client.PendingNonceAt(ctx, fromAddress)
	if err != nil {
		return nil, err
	}
	if oldNonce != 0 && !resetNonce {
		// check oldNonce to height
		// If db sets a nonce that is too high, the pending nonce will be too large
		// There is virtually no transaction in between
		// Manual handling may be required at this time
		var latestTxCount hexutil.Uint64
		err := client.Client().CallContext(ctx, &latestTxCount, "eth_getTransactionCount", fromAddress, "latest")
		if err != nil {
			return nil, err
		}
		if oldNonce > uint64(latestTxCount) {
			return nil, ErrOldNonceToHeight
		}
		nonce = oldNonce
	}

	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}

	actualGasPrice := new(big.Int).Set(gasPrice)
	b.logger.Infof("gas price:%v", new(big.Float).Quo(new(big.Float).SetInt(actualGasPrice), big.NewFloat(1e9)).String())
	b.logger.Infof("gas price:%v", actualGasPrice.String())
	b.logger.Infof("nonce:%v", nonce)
	b.logger.Infof("from address:%v", fromAddress)
	b.logger.Infof("to address:%v", toAddress.Hex())
	callMsg := ethereum.CallMsg{
		From:     fromAddress,
		To:       &toAddress,
		Value:    value,
		GasPrice: actualGasPrice,
	}
	if data != nil {
		callMsg.Data = data
		b.logger.Infof("data:%v", hexutil.Encode(data))
	}

	// use eth_estimateGas only check deposit err
	gas, err := client.EstimateGas(ctx, callMsg)
	if err != nil {
		//	// Other errors may occur that need to be handled
		//	// The estimated gas cannot block the sending of a transaction
		//	b.logger.Errorw("estimate gas err", "error", err.Error())
		//	if strings.Contains(err.Error(), ErrBridgeDepositTxHashExist.Error()) {
		//		return nil, ErrBridgeDepositTxHashExist
		//	}
		//
		//	if strings.Contains(err.Error(), ErrBridgeDepositContractInsufficientBalance.Error()) {
		//		return nil, ErrBridgeDepositContractInsufficientBalance
		//	}
		//
		//	if strings.Contains(err.Error(), ErrBridgeFromGasInsufficient.Error()) {
		//		return nil, ErrBridgeFromGasInsufficient
		//	}
		//
		//	// estimate gas err, return, try again
		//	return nil, err
		gas = 30000
	}
	gas *= 2
	legacyTx := types.LegacyTx{
		Nonce:    nonce,
		To:       &toAddress,
		Value:    value,
		Gas:      gas,
		GasPrice: actualGasPrice,
	}

	if data != nil {
		legacyTx.Data = data
	}

	tx := types.NewTx(&legacyTx)

	chainID, err := client.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	// sign tx
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), fromPriv)
	if err != nil {
		return nil, err
	}

	// send tx
	err = client.SendTransaction(ctx, signedTx)
	if err != nil {
		return nil, err
	}

	return signedTx, nil
}

func (b *Bridge) retrySendTransaction(
	ctx context.Context,
	oldTx *types.Transaction,
	fromPriv *ecdsa.PrivateKey,
	resetNonce bool,
) (*types.Transaction, error) {
	txLock.Lock()
	defer txLock.Unlock()
	client, err := ethclient.Dial(b.EthRPCURL)
	if err != nil {
		return nil, err
	}
	fromAddress := crypto.PubkeyToAddress(fromPriv.PublicKey)
	nonce := oldTx.Nonce()
	var latestTxCount hexutil.Uint64
	err = client.Client().CallContext(ctx, &latestTxCount, "eth_getTransactionCount", fromAddress, "latest")
	if err != nil {
		return nil, err
	}
	if resetNonce {
		nonce = uint64(latestTxCount)
	}
	if nonce > uint64(latestTxCount) {
		return nil, ErrOldNonceToHeight
	}

	gasPrice := oldTx.GasPrice()
	// set new gas price: newGasPrice = oldGasPrice * 2
	gasPrice.Mul(gasPrice, big.NewInt(2))

	log.Infof("new gas price:%v", new(big.Float).Quo(new(big.Float).SetInt(gasPrice), big.NewFloat(1e9)).String())
	log.Infof("new gas price:%v", gasPrice.String())
	log.Infof("nonce:%v", nonce)
	log.Infof("from address:%v", fromAddress)

	callMsg := ethereum.CallMsg{
		From:     fromAddress,
		To:       oldTx.To(),
		Value:    oldTx.Value(),
		GasPrice: gasPrice,
	}
	if oldTx.Data() != nil {
		callMsg.Data = oldTx.Data()
	}

	// use eth_estimateGas only check deposit err
	gas, err := client.EstimateGas(ctx, callMsg)
	if err != nil {
		// Other errors may occur that need to be handled
		// The estimated gas cannot block the sending of a transaction
		b.logger.Errorw("estimate gas err", "error", err.Error())
		if strings.Contains(err.Error(), ErrBridgeDepositTxHashExist.Error()) {
			return nil, ErrBridgeDepositTxHashExist
		}

		if strings.Contains(err.Error(), ErrBridgeDepositContractInsufficientBalance.Error()) {
			return nil, ErrBridgeDepositContractInsufficientBalance
		}

		if strings.Contains(err.Error(), ErrBridgeFromGasInsufficient.Error()) {
			return nil, ErrBridgeFromGasInsufficient
		}

		// estimate gas err, return, try again
		return nil, err
	}
	gas *= 2
	newlegacyTx := types.LegacyTx{
		Nonce:    nonce,
		To:       oldTx.To(),
		Value:    oldTx.Value(),
		Gas:      gas,
		GasPrice: gasPrice,
	}

	if oldTx.Data() != nil {
		newlegacyTx.Data = oldTx.Data()
	}

	tx := types.NewTx(&newlegacyTx)

	chainID, err := client.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	// sign tx
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), fromPriv)
	if err != nil {
		return nil, err
	}
	log.Infow("new tx", "tx", signedTx)
	// send tx
	err = client.SendTransaction(ctx, signedTx)
	if err != nil {
		return nil, err
	}

	return signedTx, nil
}

// ABIPack the given method name to conform the ABI. Method call's data
func (b *Bridge) ABIPack(abiData string, method string, args ...interface{}) ([]byte, error) {
	contractAbi, err := abi.JSON(bytes.NewReader([]byte(abiData)))
	if err != nil {
		return nil, err
	}
	return contractAbi.Pack(method, args...)
}

// BitcoinAddressToEthAddress bitcoin address to eth address
func (b *Bridge) BitcoinAddressToEthAddress(hash string, bitcoinAddress b2types.BitcoinFrom) (string, error) {
	pubKeyResp, err := aa.GetPubKey(b.AAPubKeyAPI, hash, bitcoinAddress.Address, b.network)
	if err != nil {
		b.logger.Errorw("Get AAAddress:", "error", err.Error())
		return "", err
	}
	//if pubkeyResp.Code != "0" {
	//	if pubkeyResp.Code == aa.AddressNotFoundErrCode {
	//		return "", ErrAAAddressNotFound
	//	}
	//	return "", fmt.Errorf("get pubkey code err:%v", pubkeyResp)
	//}
	//
	//b.logger.Infow("get pub key:", "pubkey", pubkeyResp, "address", bitcoinAddress.Address)
	//aaBtcAccount, err := b.particle.AAGetBTCAccount([]string{pubkeyResp.Data.Pubkey})
	//if err != nil {
	//	return "", err
	//}
	//
	//if len(aaBtcAccount.Result) != 1 {
	//	b.logger.Errorw("AAGetBTCAccount", "result", aaBtcAccount)
	//	return "", fmt.Errorf("AAGetBTCAccount result not match")
	//}
	//b.logger.Infow("AAGetBTCAccount", "result", aaBtcAccount.Result[0])
	//return aaBtcAccount.Result[0].SmartAccountAddress, nil

	return pubKeyResp.AAAddress, nil
}

// WaitMined wait tx mined
func (b *Bridge) WaitMined(ctx context.Context, tx *types.Transaction, _ []byte) (*types.Receipt, error) {
	client, err := ethclient.Dial(b.EthRPCURL)
	if err != nil {
		return nil, err
	}

	receipt, err := bind.WaitMined(ctx, client, tx)
	if err != nil {
		return nil, err
	}
	if receipt.Status != 1 {
		b.logger.Errorw("wait mined status err", "error", ErrBridgeWaitMinedStatus, "receipt", receipt)
		return receipt, ErrBridgeWaitMinedStatus
	}
	return receipt, nil
}

func (b *Bridge) TransactionReceipt(hash string) (*types.Receipt, error) {
	client, err := ethclient.Dial(b.EthRPCURL)
	if err != nil {
		return nil, err
	}

	receipt, err := client.TransactionReceipt(context.Background(), common.HexToHash(hash))
	if err != nil {
		return nil, err
	}
	return receipt, nil
}

func (b *Bridge) TransactionByHash(hash string) (*types.Transaction, bool, error) {
	client, err := ethclient.Dial(b.EthRPCURL)
	if err != nil {
		return nil, false, err
	}

	tx, isPending, err := client.TransactionByHash(context.Background(), common.HexToHash(hash))
	if err != nil {
		return nil, false, err
	}
	return tx, isPending, nil
}

func (b *Bridge) FromAddress() string {
	fromAddress := crypto.PubkeyToAddress(b.EthPrivKey.PublicKey)
	return fromAddress.String()
}

func has0xPrefix(input string) bool {
	return len(input) >= 2 && input[0] == '0' && (input[1] == 'x' || input[1] == 'X')
}
