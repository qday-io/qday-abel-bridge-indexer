package indexer_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path"
	"testing"

	config2 "github.com/b2network/b2-indexer/config"
	"github.com/b2network/b2-indexer/internal/logic/indexer"
	b2types "github.com/b2network/b2-indexer/internal/types"
	logger "github.com/b2network/b2-indexer/pkg/log"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newLogger(name string) logger.Logger {
	bridgeB2NodeLoggerOpt := logger.NewOptions()
	bridgeB2NodeLoggerOpt.Format = "console"
	bridgeB2NodeLoggerOpt.Level = "info"
	bridgeB2NodeLoggerOpt.EnableColor = true
	bridgeB2NodeLoggerOpt.Name = name
	bridgeB2NodeLogger := logger.New(bridgeB2NodeLoggerOpt)
	return bridgeB2NodeLogger
}

func init2(t *testing.T) *indexer.Bridge {
	abiPath := path.Join("./testdata")

	ABI := ""

	abi, err := os.ReadFile(path.Join("./testdata", "abi.json"))
	if err != nil {
		// load default abi
		ABI = config2.DefaultDepositAbi
	} else {
		ABI = string(abi)
	}

	bridgeCfg := config2.BridgeConfig{
		EthRPCURL:       "http://124.243.137.251:8123",
		ContractAddress: "0xDB6a51588433f0366082330aCFa8d2b7a1a5400A",
		EthPrivKey:      "8623eb1173b001788b7dc789513c34d049a3d02c728b50daae5799fca009e111",
		ABI:             ABI,
		AAB2PI:          "https://deposit-test.qday.ninja:9002",
	}

	log := newLogger("[bridge]")

	bridge, err := indexer.NewBridge(bridgeCfg, abiPath, log, "Abelian Testnetwork")

	if err != nil {
		t.Fatal(err)
	}

	return bridge
}

// TestLocalTransfer only test in local
func TestLocalTransfer(t *testing.T) {
	bridge := bridgeWithConfig(t)
	testCase := []struct {
		name string
		args []interface{}
		err  error
	}{
		{
			name: "success",
			args: []interface{}{
				b2types.BitcoinFrom{
					Address: "tb1qjda2l5spwyv4ekwe9keddymzuxynea2m2kj0qy",
				},
				int64(20183783146),
			},
			err: nil,
		},
		{
			name: "fail: address empty",
			args: []interface{}{
				b2types.BitcoinFrom{},
				int64(1234),
			},
			err: errors.New("bitcoin address is empty"),
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			hex, _, err := bridge.Transfer(tc.args[0].(b2types.BitcoinFrom), tc.args[1].(int64), nil, 0, false)
			if err != nil {
				assert.Equal(t, tc.err, err)
			}
			t.Log(hex)
		})
	}
}

func TestABIPack(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		abiData, err := os.ReadFile(path.Join("./testdata", "abi.json"))
		if err != nil {
			t.Fatal(err)
		}
		expectedMethod := "deposit"
		expectedArgs := []interface{}{common.HexToAddress("0x12345678"), new(big.Int).SetInt64(1111)}
		expectedResult := []byte{
			71, 231, 239, 36, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 18, 52, 86, 120, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 4, 87,
		}

		// Create a mock bridge object
		b := &indexer.Bridge{}

		// Call the ABIPack method
		result, err := b.ABIPack(string(abiData), expectedMethod, expectedArgs...)
		// Check for errors
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		// Compare the result with the expected result
		require.Equal(t, result, expectedResult)
	})

	t.Run("Invalid ABI data", func(t *testing.T) {
		abiData := `{"inputs": [{"type": "address", "name": "to"}, {"type": "uint256", "name": "value"}`
		expectedError := errors.New("unexpected EOF")

		// Create a mock bridge object
		b := &indexer.Bridge{}

		// Call the ABIPack method
		_, err := b.ABIPack(abiData, "method", "arg1", "arg2")

		require.EqualError(t, err, expectedError.Error())
	})
}

func bridgeWithConfig(t *testing.T) *indexer.Bridge {
	config, err := config2.LoadBitcoinConfig("")
	require.NoError(t, err)
	bridge, err := indexer.NewBridge(config.Bridge, "./", logger.NewNopLogger(), chaincfg.TestNet3Params.Name)
	require.NoError(t, err)
	return bridge
}

// func TestLocalTransactionByHash(t *testing.T) {
// 	bridge := bridgeWithConfig(t)

// 	tx, pending, err := bridge.TransactionByHash("0xaa0d1b59f1834becb63f982b4712f848402b2d577bf74bfbcf402d63a9d460d9")
// 	if err != nil {
// 		t.Fail()
// 	}

// 	// v, r, s := tx.RawSignatureValues()

// 	// fmt.Println(tx, pending, v.String(), r.String(), s.String())
// 	t.Fail()
// }

func randHash(t *testing.T) string {
	randomTx := wire.NewMsgTx(wire.TxVersion)
	randomData := make([]byte, 32)
	_, err := rand.Read(randomData)
	assert.NoError(t, err)
	randomTx.AddTxOut(&wire.TxOut{
		PkScript: randomData,
		Value:    0,
	})
	return randomTx.TxHash().String()
}

// TestLocalBatchTransferWaitMined
// Using this test method, you can batch send transactions to consume nonce
func TestLocalBatchRestNonce(t *testing.T) {
	config, err := config2.LoadBitcoinConfig("")
	require.NoError(t, err)
	// custom rpc key gas price
	// config.Bridge.GasPriceMultiple = 3
	// config.Bridge.EthRPCURL = ""
	// config.Bridge.EthPrivKey = ""
	bridge, err := indexer.NewBridge(config.Bridge, "./", logger.NewNopLogger(), chaincfg.TestNet3Params.Name)
	privateKey, err := crypto.HexToECDSA(config.Bridge.EthPrivKey)
	require.NoError(t, err)
	ctx := context.Background()
	fromAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	client, err := ethclient.Dial(config.Bridge.EthRPCURL)
	require.NoError(t, err)
	// pending nonce
	pendingnonce, err := client.PendingNonceAt(ctx, fromAddress)
	require.NoError(t, err)
	// latest nonce
	var latestResult hexutil.Uint64
	err = client.Client().CallContext(ctx, &latestResult, "eth_getTransactionCount", fromAddress, "latest")
	require.NoError(t, err)
	latestNonce := uint64(latestResult)
	if latestNonce == pendingnonce {
		return
	}
	for i := latestNonce; i < pendingnonce; i++ {
		// normal
		b2Tx, err := testSendTransaction(ctx, privateKey, fromAddress, i, config.Bridge)
		if err != nil {
			assert.NoError(t, err)
		}
		_, err = bridge.WaitMined(context.Background(), b2Tx, nil)
		if err != nil {
			assert.NoError(t, err)
		}
		fmt.Println(b2Tx.Hash())
	}
}

func testSendTransaction(ctx context.Context, fromPriv *ecdsa.PrivateKey,
	toAddress common.Address, oldNonce uint64, cfg config2.BridgeConfig,
) (*types.Transaction, error) {
	client, err := ethclient.Dial(cfg.EthRPCURL)
	if err != nil {
		return nil, err
	}
	fromAddress := crypto.PubkeyToAddress(fromPriv.PublicKey)
	nonce, err := client.PendingNonceAt(ctx, fromAddress)
	if err != nil {
		return nil, err
	}
	if oldNonce != 0 {
		nonce = oldNonce
	}
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}
	gasPrice.Mul(gasPrice, big.NewInt(cfg.GasPriceMultiple))

	actualGasPrice := new(big.Int).Set(gasPrice)
	logger.Infof("gas price:%v", new(big.Float).Quo(new(big.Float).SetInt(actualGasPrice), big.NewFloat(1e9)).String())
	logger.Infof("gas price:%v", actualGasPrice.String())
	logger.Infof("nonce:%v", nonce)
	logger.Infof("from address:%v", fromAddress)
	logger.Infof("to address:%v", toAddress)
	callMsg := ethereum.CallMsg{
		From:     fromAddress,
		To:       &toAddress,
		GasPrice: actualGasPrice,
	}

	// use eth_estimateGas only check deposit err
	gas, err := client.EstimateGas(ctx, callMsg)
	if err != nil {
		// estimate gas err, return, try again
		return nil, err
	}
	gas *= 2
	legacyTx := types.LegacyTx{
		Nonce:    nonce,
		To:       &toAddress,
		Gas:      gas,
		GasPrice: actualGasPrice,
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

func TestBridge_Deposit(t *testing.T) {
	b := init2(t)
	hash := "4cd7a9634158d12e21bc5ce9aae0f36299a1cfc775c5b378f72876d003b6cedf"
	//hex.EncodeToString()
	from := b2types.BitcoinFrom{
		Address: "abe36f503e14f9fe13950e009d89de269031aab054223858cc4241224b95c9fd0bed381d445ca1077b69f4bd12faa2248797f6edaee7d4777ff1a6366f3a46d198d8",
	}

	client, err := ethclient.Dial(b.EthRPCURL)
	if err != nil {
		panic(err)
	}
	fromAddress := crypto.PubkeyToAddress(b.EthPrivKey.PublicKey)
	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)

	logger.Infof("from address:%v", fromAddress.Hex(), nonce)

	tos := "[{\"Value\": 16, \"Address\": \"0x1111111254fb6c44bAC0beD2854e76F90643097d\"}]"

	b2Tx, _, aaAddress, fromAddr, err := b.Deposit(hash, from, tos, 150000000000, nil, nonce, false)

	if err != nil {
		t.Fatal(err)
	}

	t.Logf("b2tx:%v", b2Tx.Hash().String())
	t.Logf("receiptAddress:%v \n", aaAddress)
	t.Logf("fromAddr:%v\n", fromAddr)
}
