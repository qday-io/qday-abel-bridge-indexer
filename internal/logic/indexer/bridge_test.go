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
	"time"

	"github.com/b2network/b2-indexer/internal/config"
	"github.com/b2network/b2-indexer/internal/logic/indexer"
	b2types "github.com/b2network/b2-indexer/internal/types"
	"github.com/b2network/b2-indexer/pkg/log"
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

func TestNewBridge(t *testing.T) {
	abiPath := path.Join("./testdata")

	ABI := ""

	abi, err := os.ReadFile(path.Join("./testdata", "abi.json"))
	if err != nil {
		// load default abi
		ABI = config.DefaultDepositAbi
	} else {
		ABI = string(abi)
	}

	privateKey, err := crypto.HexToECDSA("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}

	bridgeCfg := config.BridgeConfig{
		EthRPCURL:           "http://localhost:8545",
		ContractAddress:     "0x123456789abcdef",
		EthPrivKey:          "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ABI:                 "abi.json",
		AAParticleRPC:       "http://localhost:8545",
		AAParticleProjectID: "1111",
		AAParticleServerKey: "",
		AAParticleChainID:   1102,
	}

	bridge, err := indexer.NewBridge(bridgeCfg, abiPath, log.NewNopLogger(), &chaincfg.TestNet3Params)
	assert.NoError(t, err)
	assert.NotNil(t, bridge)
	assert.Equal(t, bridgeCfg.EthRPCURL, bridge.EthRPCURL)
	assert.Equal(t, common.HexToAddress("0x123456789abcdef"), bridge.ContractAddress)
	assert.Equal(t, privateKey, bridge.EthPrivKey)
	assert.Equal(t, ABI, bridge.ABI)
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

// TestLocalBitcoinAddressToEthAddress only test in local
func TestLocalBitcoinAddressToEthAddress(t *testing.T) {
	bridge := bridgeWithConfig(t)
	testCase := []struct {
		name           string
		bitcoinAddress b2types.BitcoinFrom
		wantErr        bool
	}{
		{
			name: "success",
			bitcoinAddress: b2types.BitcoinFrom{
				Address: "1McUczq9Cq8DL1YwaQCr6nSseuEBkpQdBh",
			},
			wantErr: false,
		},
		{
			name: "pubkey fail",
			bitcoinAddress: b2types.BitcoinFrom{
				Address: "1McUczq9Cq8DL1YwaQCr6nSseuEBkpQdBh",
			},
			wantErr: true,
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			ethAddress, err := bridge.BitcoinAddressToEthAddress(tc.bitcoinAddress)
			if (err != nil) != tc.wantErr {
				t.Errorf("TestLocalBitcoinAddressToEthAddress() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if !common.IsHexAddress(ethAddress) {
				t.Errorf("bitcoinAddress: %s, ethAddress: %s", tc.bitcoinAddress, ethAddress)
			}
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
	config, err := config.LoadBitcoinConfig("")
	require.NoError(t, err)
	bridge, err := indexer.NewBridge(config.Bridge, "./", log.NewNopLogger(), &chaincfg.TestNet3Params)
	require.NoError(t, err)
	return bridge
}

func TestLocalDepositWaitMined(t *testing.T) {
	bridge := bridgeWithConfig(t)
	uuid := randHash(t)
	address := b2types.BitcoinFrom{
		Address: "tb1qjda2l5spwyv4ekwe9keddymzuxynea2m2kj0qy1",
	}
	value := 123
	bigValue := 11111111111111111

	// params check
	_, _, _, _, err := bridge.Deposit("", address, int64(value), nil, 0, false)
	if err != nil {
		assert.EqualError(t, errors.New("tx id is empty"), err.Error())
	}
	_, _, _, _, err = bridge.Deposit(uuid, b2types.BitcoinFrom{}, int64(value), nil, 0, false)
	if err != nil {
		assert.EqualError(t, errors.New("bitcoin address is empty"), err.Error())
	}

	// normal
	b2Tx, _, _, _, err := bridge.Deposit(uuid, address, int64(value), nil, 0, false)
	if err != nil {
		assert.NoError(t, err)
	}
	_, err = bridge.WaitMined(context.Background(), b2Tx, nil)
	if err != nil {
		assert.NoError(t, err)
	}

	// uuid check
	_, _, _, _, err = bridge.Deposit(uuid, address, int64(value), nil, 0, false)
	if err != nil {
		assert.EqualError(t, indexer.ErrBridgeDepositTxHashExist, err.Error())
	}

	// insufficient balance
	_, _, _, _, err = bridge.Deposit(randHash(t), address, int64(bigValue), nil, 0, false)
	if err != nil {
		assert.EqualError(t, indexer.ErrBridgeDepositContractInsufficientBalance, err.Error())
	} else {
		t.Fatal("insufficient balance check failed")
	}

	// context timeout
	b2Tx2, _, _, _, err := bridge.Deposit(randHash(t), address, int64(value), nil, 0, false)
	if err != nil {
		assert.NoError(t, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Millisecond)
	defer cancel()
	_, err = bridge.WaitMined(ctx, b2Tx2, nil)
	if err != nil {
		assert.EqualError(t, context.DeadlineExceeded, err.Error())
	} else {
		t.Fatal("context deadline check failed")
	}
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
	config, err := config.LoadBitcoinConfig("")
	require.NoError(t, err)
	config.Bridge.EnableVSM = false
	// custom rpc key gas price
	// config.Bridge.GasPriceMultiple = 3
	// config.Bridge.EthRPCURL = ""
	// config.Bridge.EthPrivKey = ""
	bridge, err := indexer.NewBridge(config.Bridge, "./", log.NewNopLogger(), &chaincfg.TestNet3Params)
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
	toAddress common.Address, oldNonce uint64, cfg config.BridgeConfig,
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
	log.Infof("gas price:%v", new(big.Float).Quo(new(big.Float).SetInt(actualGasPrice), big.NewFloat(1e9)).String())
	log.Infof("gas price:%v", actualGasPrice.String())
	log.Infof("nonce:%v", nonce)
	log.Infof("from address:%v", fromAddress)
	log.Infof("to address:%v", toAddress)
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
