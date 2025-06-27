package handler

import (
	osContext "context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/qday-io/qday-abel-bridge-indexer/config"
	"github.com/qday-io/qday-abel-bridge-indexer/internal/logic/indexer"
	"github.com/qday-io/qday-abel-bridge-indexer/internal/model"
	"github.com/qday-io/qday-abel-bridge-indexer/internal/types"
	logger "github.com/qday-io/qday-abel-bridge-indexer/pkg/log"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

func HandleIndexCmd(ctx *model.Context, cmd *cobra.Command) (err error) {
	//home := ctx.Config.RootDir
	bitcoinCfg := ctx.BitcoinConfig
	context, cancel := osContext.WithCancel(osContext.Background())
	defer cancel()
	if bitcoinCfg.EnableIndexer {
		err = runIndexerService(ctx, cmd, context)
		if err != nil {
			return err
		}
	}

	//if bitcoinCfg.Bridge.EnableRollupListener {
	//	err = runRollupListenerService(ctx, cmd)
	//	if err != nil {
	//		return err
	//	}
	//}

	//if bitcoinCfg.Bridge.EnableWithdrawListener {
	//	err = runWithDrawService(ctx, cmd)
	//	if err != nil {
	//		return err
	//	}
	//}

	// wait quit
	code := WaitForQuitSignals()
	logger.Infow("server stop!!!", "quit code", code)
	return nil
}

func runIndexerService(ctx *model.Context, cmd *cobra.Command, context osContext.Context) error {
	//home := ctx.Config.RootDir
	bitcoinCfg := ctx.BitcoinConfig
	logger.Infow("bitcoin index service starting!!!")

	bidxLogger := newLogger(ctx, "[bitcoin-indexer]")
	//bidxer, err := indexer.NewBitcoinIndexer(bidxLogger, ctx, bitcoinCfg.IndexerListenAddress, bitcoinCfg.IndexerListenTargetConfirmations)

	bidxer, err := indexer.NewAbelianIndexer(bidxLogger, bitcoinCfg, bitcoinCfg.IndexerListenAddress, bitcoinCfg.IndexerListenTargetConfirmations)
	if err != nil {
		logger.Errorw("failed to new bitcoin indexer indexer", "error", err.Error())
		return err
	}

	defer func() {
		bidxer.Stop()
	}()

	//go func() {
	err = startIndexProvider(bidxer, bidxLogger, cmd)
	if err != nil {
		return err
	}
	//}()

	<-time.After(2 * time.Second) // assume server started successfully

	if bidxer == nil {
		return fmt.Errorf("failed to new bitcoin indexer indexer:%v", err.Error())
	}

	// start l1->l2 bridge service
	//go func() {
	err = startBridgeProvider(ctx, bitcoinCfg, context, bidxer, cmd)
	if err != nil {
		return err
	}
	//}()

	<-context.Done()
	return nil
}

func startBridgeProvider(ctx *model.Context, bitcoinCfg *config.BitcoinConfig, context osContext.Context, bidxer types.BitcoinTxIndexer, cmd *cobra.Command) error {
	home := ctx.Config.RootDir
	//bitcoinParam := config.ChainParams(bitcoinCfg.NetworkName)
	db, err := GetDBContextFromCmd(cmd)
	if err != nil {
		logger.Errorw("failed to get db context", "error", err.Error())
		return err
	}
	bridgeLogger := newLogger(ctx, "[bridge-deposit]")
	bridge, err := indexer.NewBridge(bitcoinCfg.Bridge, home, bridgeLogger, bitcoinCfg.NetworkName)
	if err != nil {
		logger.Errorw("failed to create bitcoin bridge", "error", err.Error())
		return err
	}

	bridgeService := indexer.NewBridgeDepositService(bridge, bidxer, db, bridgeLogger, bitcoinCfg.Bridge)
	bridgeErrCh := make(chan error)
	go func() {
		if err := bridgeService.Start(); err != nil {
			bridgeErrCh <- err
			//return err
		}
	}()

	select {
	case err := <-bridgeErrCh:
		return err
	case <-context.Done():

	}

	defer func() {
		if err = bridgeService.Stop(); err != nil {
			logger.Errorf("stop err:%v", err.Error())
		}
	}()

	return nil
}

func startIndexProvider(bidxer types.BitcoinTxIndexer, bidxLogger logger.Logger, cmd *cobra.Command) error {

	//bitcoinParam := config.ChainParams(bitcoinCfg.NetworkName)
	//bidxLogger := newLogger(ctx, "[bitcoin-indexer]")
	//bidxer, err := indexer.NewBitcoinIndexer(bidxLogger, bclient, bitcoinParam, bitcoinCfg.IndexerListenAddress, bitcoinCfg.IndexerListenTargetConfirmations)
	//if err != nil {
	//	logger.Errorw("failed to new bitcoin indexer indexer", "error", err.Error())
	//	return nil, err
	//}
	// check bitcoin core status, whether the request succeed
	_, err := bidxer.BlockChainInfo()
	if err != nil {
		logger.Errorw("failed to get bitcoin core status", "error", err.Error())
		return err
	}

	db, err := GetDBContextFromCmd(cmd)
	if err != nil {
		logger.Errorw("failed to get db context", "error", err.Error())
		return err
	}

	bindexerService := indexer.NewIndexerService(bidxer, db, bidxLogger)

	err = bindexerService.CheckDb()
	if err != nil {
		logger.Errorw("failed to get db context", "error", err.Error())
		return err
	}

	errCh := make(chan error)
	go func() {
		if err := bindexerService.Start(); err != nil {
			if err != nil {
				errCh <- err
			}
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-time.After(5 * time.Second): // assume server started successfully
	}
	return nil
}

func runEpsService(ctx *model.Context, cmd *cobra.Command) error {
	//	epsLoggerOpt := logger.NewOptions()
	//	epsLoggerOpt.Format = ctx.Config.LogFormat
	//	epsLoggerOpt.Level = ctx.Config.LogLevel
	//	epsLoggerOpt.EnableColor = true
	//	epsLoggerOpt.Name = "[eps]"
	//	epsLogger := logger.New(epsLoggerOpt)
	//
	//	db, err := GetDBContextFromCmd(cmd)
	//	if err != nil {
	//		logger.Errorw("failed to get db context", "error", err.Error())
	//		return err
	//	}
	//
	//	epsService, err := bitcoin.NewEpsService(bitcoinCfg.Bridge, bitcoinCfg.Eps, epsLogger, db)
	//	if err != nil {
	//		logger.Errorw("failed to new eps server", "error", err.Error())
	//		return err
	//	}
	//	epsErrCh := make(chan error)
	//	go func() {
	//		if err := epsService.OnStart(); err != nil {
	//			epsErrCh <- err
	//		}
	//	}()
	//
	//	select {
	//	case err := <-epsErrCh:
	//		return err
	//	case <-time.After(5 * time.Second): // assume server started successfully
	//	}
	return nil
}

func runRollupListenerService(ctx *model.Context, cmd *cobra.Command) error {
	//	logger.Infow("rollup indexer service starting...")
	//	db, err := GetDBContextFromCmd(cmd)
	//	if err != nil {
	//		logger.Errorw("failed to get db context", "error", err.Error())
	//		return err
	//	}
	//
	//	//btclient, err := rpcclient.New(&rpcclient.ConnConfig{
	//	//	Host:         bitcoinCfg.RPCHost + ":" + bitcoinCfg.RPCPort,
	//	//	User:         bitcoinCfg.RPCUser,
	//	//	Pass:         bitcoinCfg.RPCPass,
	//	//	HTTPPostMode: true, // Bitcoin core only supports HTTP POST mode
	//	//	DisableTLS:   true, // Bitcoin core does not provide TLS by default
	//	//}, nil)
	//	//if err != nil {
	//	//	logger.Errorw("EVMListenerService failed to create bitcoin client", "error", err.Error())
	//	//	return err
	//	//}
	//	//defer func() {
	//	//	btclient.Shutdown()
	//	//}()
	//
	//	ethlient, err := ethclient.Dial(bitcoinCfg.Bridge.EthRPCURL)
	//	if err != nil {
	//		logger.Errorw("EVMListenerService failed to create eth client", "error", err.Error())
	//		return err
	//	}
	//	defer func() {
	//		ethlient.Close()
	//	}()
	//
	//	rollupLogger := newLogger(ctx, "[rollup-service]")
	//	if err != nil {
	//		return err
	//	}
	//	rollupService := rollup.NewIndexerService(ethlient, bitcoinCfg, db, rollupLogger)
	//
	//	epsErrCh := make(chan error)
	//	go func() {
	//		if err := rollupService.OnStart(); err != nil {
	//			epsErrCh <- err
	//		}
	//	}()
	//
	//	select {
	//	case err := <-epsErrCh:
	//		return err
	//	case <-time.After(5 * time.Second): // assume server started successfully
	//	}
	return nil
}

func runWithDrawService(ctx *model.Context, cmd *cobra.Command) error {
	//	logger.Infow("withdraw service starting...")
	//	db, err := GetDBContextFromCmd(cmd)
	//	if err != nil {
	//		logger.Errorw("failed to get db context", "error", err.Error())
	//		return err
	//	}
	//
	//	btclient, err := rpcclient.New(&rpcclient.ConnConfig{
	//		Host:         bitcoinCfg.RPCHost + ":" + bitcoinCfg.RPCPort,
	//		User:         bitcoinCfg.RPCUser,
	//		Pass:         bitcoinCfg.RPCPass,
	//		HTTPPostMode: true, // Bitcoin core only supports HTTP POST mode
	//		DisableTLS:   true, // Bitcoin core does not provide TLS by default
	//	}, nil)
	//	if err != nil {
	//		logger.Errorw("EVMListenerService failed to create bitcoin client", "error", err.Error())
	//		return err
	//	}
	//	defer func() {
	//		btclient.Shutdown()
	//	}()
	//
	//	ethlient, err := ethclient.Dial(bitcoinCfg.Bridge.EthRPCURL)
	//	if err != nil {
	//		logger.Errorw("EVMListenerService failed to create eth client", "error", err.Error())
	//		return err
	//	}
	//	defer func() {
	//		ethlient.Close()
	//	}()
	//
	//	bridgeLogger := newLogger(ctx, "[bridge-withdraw]")
	//	if err != nil {
	//		return err
	//	}
	//	withdrawService := bitcoin.NewBridgeWithdrawService(btclient, ethlient, bitcoinCfg, db, bridgeLogger)
	//
	//	epsErrCh := make(chan error)
	//	go func() {
	//		if err := withdrawService.OnStart(); err != nil {
	//			epsErrCh <- err
	//		}
	//	}()
	//
	//	select {
	//	case err := <-epsErrCh:
	//		return err
	//	case <-time.After(5 * time.Second): // assume server started successfully
	//	}
	return nil
}

func GetDBContextFromCmd(cmd *cobra.Command) (*gorm.DB, error) {
	if v := cmd.Context().Value(types.DBContextKey); v != nil {
		db := v.(*gorm.DB)
		return db, nil
	}
	return nil, fmt.Errorf("db context not set")
}

func WaitForQuitSignals() int {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT, syscall.SIGHUP)
	sig := <-sigs
	return int(sig.(syscall.Signal)) + 128
}

func newLogger(ctx *model.Context, name string) logger.Logger {
	bridgeB2NodeLoggerOpt := logger.NewOptions()
	bridgeB2NodeLoggerOpt.Format = ctx.Config.LogFormat
	bridgeB2NodeLoggerOpt.Level = ctx.Config.LogLevel
	bridgeB2NodeLoggerOpt.EnableColor = true
	bridgeB2NodeLoggerOpt.Name = name
	bridgeB2NodeLogger := logger.New(bridgeB2NodeLoggerOpt)
	return bridgeB2NodeLogger
}
