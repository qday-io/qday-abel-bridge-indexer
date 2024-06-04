package indexer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/b2network/b2-indexer/internal/config"
	"github.com/b2network/b2-indexer/internal/model"
	"github.com/b2network/b2-indexer/internal/types"
	"github.com/b2network/b2-indexer/pkg/log"
	"github.com/cometbft/cometbft/libs/service"
	"github.com/ethereum/go-ethereum"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"gorm.io/gorm"
)

const (
	BridgeDepositServiceName = "BitcoinBridgeDepositService"
	BatchDepositWaitTimeout  = 10 * time.Second
	DepositErrTimeout        = 20 * time.Second
	BatchDepositLimit        = 100
	WaitMinedTimeout         = 20 * time.Second
	HandleDepositTimeout     = 1 * time.Second
	DepositRetry             = 10 // temp fix, Increase retry times
)

var ErrServerStop = errors.New("server stop")

// BridgeDepositService l1->l2
type BridgeDepositService struct {
	service.BaseService
	bridgeCfg  config.BridgeConfig
	bridge     types.BITCOINBridge
	btcIndexer types.BITCOINTxIndexer
	db         *gorm.DB
	log        log.Logger
	wg         sync.WaitGroup
	stopChan   chan struct{}
}

// NewBridgeDepositService returns a new service instance.
func NewBridgeDepositService(
	bridge types.BITCOINBridge,
	btcIndexer types.BITCOINTxIndexer,
	db *gorm.DB,
	logger log.Logger,
	bridgeCfg config.BridgeConfig,
) *BridgeDepositService {
	is := &BridgeDepositService{
		bridge:     bridge,
		btcIndexer: btcIndexer,
		db:         db,
		log:        logger,
		bridgeCfg:  bridgeCfg,
	}
	is.BaseService = *service.NewBaseService(nil, BridgeDepositServiceName, is)
	return is
}

// OnStart
func (bis *BridgeDepositService) OnStart() error {
	bis.wg.Add(2)

	go bis.Deposit()

	if bis.bridgeCfg.EnableRollupListener {
		go bis.CheckDeposit()
	}
	bis.stopChan = make(chan struct{})
	select {}
}

func (bis *BridgeDepositService) OnStop() {
	bis.log.Warnf("bridge deposit service stoping...")
	close(bis.stopChan)
	bis.wg.Wait()
}

func (bis *BridgeDepositService) Deposit() {
	//ticker := time.NewTicker(BatchDepositWaitTimeout)
	defer func() {
		bis.wg.Done()
		//ticker.Stop()
	}()
	for {
		//DEPOSIT:
		select {
		case <-bis.stopChan:
			bis.log.Warnf("deposit stopping...")
			return
		case <-time.After(BatchDepositWaitTimeout):
			// Priority processing UnconfirmedDeposit
			err := bis.UnconfirmedDeposit()
			if err != nil {
				bis.log.Warnf("unconfirmed deposit err: %s", err)
				if errors.Is(err, ErrServerStop) {
					return
				}
				continue
			}

			// Query condition
			// 1. tx status is pending
			// 2. contract insufficient balance
			// 3. invoke contract from account insufficient balance
			// 4. callback status is success
			// 5. listener status is success
			//var deposits []model.Deposit
			deposits, err := bis.GetTxToDeposit()

			if err != nil {
				bis.log.Errorw("failed find tx from db", "error", err)
			}

			bis.log.Infow("start handle deposit", "deposit batch num", len(deposits))
			for _, deposit := range deposits {
				err = bis.HandleDeposit(deposit, nil, deposit.B2TxNonce, false)
				if err != nil {
					bis.log.Errorw("handle deposit failed", "error", err, "deposit", deposit)
					if errors.Is(err, ErrServerStop) {
						return
					}
					//break DEPOSIT
					//todo  add feature by vike
					continue
				}
				select {
				case <-bis.stopChan:
					bis.log.Warnf("handle deposit stopping...")
					return
				case <-time.After(HandleDepositTimeout):
				}
			}

			// handle aa not found err
			// If there is no binding between the registered address and pubkey
			// an error will occur, which can be handled again next time
			var aaNotFoundDeposits []*model.Deposit
			err = bis.db.Where(fmt.Sprintf("%s.%s = ?", model.Deposit{}.TableName(), model.Deposit{}.Column().B2TxStatus), model.DepositB2TxStatusAAAddressNotFound).
				Limit(BatchDepositLimit).
				Order(fmt.Sprintf("%s.%s ASC", model.Deposit{}.TableName(), model.Deposit{}.Column().BtcBlockNumber)).
				Order(fmt.Sprintf("%s.%s ASC", model.Deposit{}.TableName(), "id")).
				Find(&aaNotFoundDeposits).Error
			if err != nil {
				bis.log.Errorw("failed find tx from db", "error", err)
			}

			bis.log.Infow("start handle aa not found deposit", "aa not found deposit batch num", len(aaNotFoundDeposits))
			for _, deposit := range aaNotFoundDeposits {
				err = bis.HandleDeposit(deposit, nil, deposit.B2TxNonce, false)
				if err != nil {
					if errors.Is(err, ErrAAAddressNotFound) {
						bis.log.Warnf("aa address not found")
					} else {
						bis.log.Errorw("handle aa not found deposit failed", "error", err, "deposit", deposit)
					}
					if errors.Is(err, ErrServerStop) {
						return
					}
					//break DEPOSIT
					//todo  add feature by vike
					continue
				}
				select {
				case <-bis.stopChan:
					bis.log.Warnf("handle aa not found deposit stopping...")
					return
				case <-time.After(HandleDepositTimeout):
				}
			}
		}
	}
}

func (bis *BridgeDepositService) GetTxToDeposit() ([]*model.Deposit, error) {

	// Query condition
	// 1. tx status is pending
	// 2. contract insufficient balance
	// 3. invoke contract from account insufficient balance
	// 4. callback status is success
	// 5. listener status is success
	var deposits []*model.Deposit
	err := bis.db.
		Where(
			fmt.Sprintf("%s.%s IN (?)", model.Deposit{}.TableName(), model.Deposit{}.Column().B2TxStatus),
			[]int{
				model.DepositB2TxStatusPending,
				model.DepositB2TxStatusInsufficientBalance,
				model.DepositB2TxStatusFromAccountGasInsufficient,
			},
		).
		Where(
			fmt.Sprintf("%s.%s = ?", model.Deposit{}.TableName(), model.Deposit{}.Column().CallbackStatus),
			model.CallbackStatusSuccess,
		).
		Where(
			fmt.Sprintf("%s.%s = ?", model.Deposit{}.TableName(), model.Deposit{}.Column().ListenerStatus),
			model.ListenerStatusSuccess,
		).
		Limit(BatchDepositLimit).
		Order(fmt.Sprintf("%s.%s ASC", model.Deposit{}.TableName(), model.Deposit{}.Column().BtcBlockNumber)).
		Order(fmt.Sprintf("%s.%s ASC", model.Deposit{}.TableName(), "id")).
		Find(&deposits).Error

	if err != nil || len(deposits) == 0 {
		return nil, err
	}
	return deposits, nil
}

func (bis *BridgeDepositService) UnconfirmedDeposit() error {
	var deposits []*model.Deposit
	err := bis.db.
		Where(
			fmt.Sprintf("%s.%s IN (?)", model.Deposit{}.TableName(), model.Deposit{}.Column().B2TxStatus),
			[]int{
				model.DepositB2TxStatusContextDeadlineExceeded,
				model.DepositB2TxStatusWaitMined,
				model.DepositB2TxStatusWaitMinedFailed,
				model.DepositB2TxStatusIsPending,
				model.DepositB2TxStatusNonceToLow,
			},
		).
		Order(fmt.Sprintf("%s.%s ASC", model.Deposit{}.TableName(), model.Deposit{}.Column().B2TxNonce)).
		Find(&deposits).Error
	if err != nil {
		bis.log.Errorw("failed find tx from db", "error", err)
		return err
	}

	bis.log.Infow("start handle unconfirmed deposit", "unconfirmed deposit batch num", len(deposits))
	for _, deposit := range deposits {
		err = bis.HandleUnconfirmedDeposit(deposit)
		if err != nil {
			bis.log.Errorw("handle unconfirmed failed", "error", err, "deposit", deposit)
			return err
		}
		select {
		case <-bis.stopChan:
			bis.log.Warnf("unconfirmed deposit stopping...")
			return ErrServerStop
		case <-time.After(HandleDepositTimeout):
		}
	}
	return nil
}

func (bis *BridgeDepositService) HandleDeposit(deposit *model.Deposit, oldTx *ethTypes.Transaction, nonce uint64, resetNonce bool) error {
	defer func() {
		if err := recover(); err != nil {
			bis.log.Errorw("panic err", err)
		}
	}()

	if oldTx != nil {
		bis.log.Warnw("handle old deposit", "old tx:", oldTx)
	}

	// check Confirmations
	err := bis.btcIndexer.CheckConfirmations(deposit.BtcTxHash)
	if err != nil {
		bis.log.Errorw("check btc tx confirmations err", "tx hash:", deposit.B2TxHash, "err:", err)
		return err
	}

	// send deposit tx
	b2Tx, _, aaAddress, fromAddress, err := bis.bridge.Deposit(deposit.BtcTxHash, types.BitcoinFrom{
		Address: deposit.BtcFrom,
	}, deposit.BtcTos, deposit.BtcValue, oldTx, nonce, resetNonce)
	if err != nil {
		switch {
		case errors.Is(err, ErrBridgeDepositTxHashExist):
			deposit.B2TxStatus = model.DepositB2TxStatusTxHashExist
			bis.log.Errorw("invoke deposit send tx hash exist",
				"error", err.Error(),
				"btcTxHash", deposit.BtcTxHash,
				"data", deposit)
			if deposit.B2TxHash != "" {
				receipt, err := bis.bridge.TransactionReceipt(deposit.B2TxHash)
				if err == nil {
					if receipt.Status == 1 {
						deposit.B2TxStatus = model.DepositB2TxStatusSuccess
					} else {
						deposit.B2TxStatus = model.DepositB2TxStatusWaitMinedStatusFailed
					}
				} else {
					bis.log.Errorw("transaction receipt err", "error", err.Error())
				}
			}
		case errors.Is(err, ErrBridgeDepositContractInsufficientBalance):
			deposit.B2TxStatus = model.DepositB2TxStatusInsufficientBalance
			bis.log.Errorw("invoke deposit send tx contract insufficient balance",
				"error", err.Error(),
				"btcTxHash", deposit.BtcTxHash,
				"data", deposit)
		case errors.Is(err, ErrBridgeFromGasInsufficient):
			deposit.B2TxStatus = model.DepositB2TxStatusFromAccountGasInsufficient
			bis.log.Errorw("invoke deposit send tx from account gas insufficient",
				"error", err.Error(),
				"btcTxHash", deposit.BtcTxHash,
				"data", deposit)
		case errors.Is(err, ErrAAAddressNotFound):
			deposit.B2TxStatus = model.DepositB2TxStatusAAAddressNotFound
			bis.log.Warnw("invoke deposit send tx aa address not found",
				"error", err.Error(),
				"btcTxHash", deposit.BtcTxHash,
				"data", deposit)
		case strings.Contains(err.Error(), "already known"):
			bis.log.Errorw("invoke deposit send tx already known",
				"error", err.Error(),
				"btcTxHash", deposit.BtcTxHash,
				"data", deposit)
			if deposit.B2TxHash != "" {
				deposit.B2TxStatus = model.DepositB2TxStatusIsPending
				bis.log.Infof("b2 tx hash not empty, set status to ispending")
			}
		case strings.Contains(err.Error(), "nonce too low"):
			deposit.B2TxStatus = model.DepositB2TxStatusNonceToLow
			bis.log.Errorw("invoke deposit send tx nonce to low",
				"error", err.Error(),
				"btcTxHash", deposit.BtcTxHash,
				"data", deposit)
		default:
			deposit.B2TxRetry++
			deposit.B2TxStatus = model.DepositB2TxStatusPending
			bis.log.Errorw("invoke deposit send tx retry",
				"error", err.Error(),
				"btcTxHash", deposit.BtcTxHash,
				"data", deposit)
			// The call may not succeed due to network reasons. sleep wait for a while
			err = bis.db.Model(&model.Deposit{}).Where("id = ?", deposit.ID).Updates(map[string]interface{}{
				model.Deposit{}.Column().B2TxStatus: deposit.B2TxStatus,
				model.Deposit{}.Column().B2TxRetry:  deposit.B2TxRetry,
			}).Error
			if err != nil {
				return err
			}
			//tryTicker := time.NewTicker(DepositErrTimeout)
			//select {
			//case <-bis.stopChan:
			//	return ErrServerStop
			//case <-tryTicker.C:
			//	return fmt.Errorf("retry handle deposit")
			//}
		}
		dbErr := bis.db.Model(&model.Deposit{}).Where("id = ?", deposit.ID).Updates(map[string]interface{}{
			model.Deposit{}.Column().B2TxStatus: deposit.B2TxStatus,
		}).Error
		if dbErr != nil {
			return dbErr
		}
		return err
	}

	deposit.B2TxStatus = model.DepositB2TxStatusWaitMined
	deposit.B2TxHash = b2Tx.Hash().String()
	deposit.BtcFromAAAddress = aaAddress
	deposit.B2TxNonce = b2Tx.Nonce()
	updateFields := map[string]interface{}{
		model.Deposit{}.Column().B2TxHash:         deposit.B2TxHash,
		model.Deposit{}.Column().BtcFromAAAddress: deposit.BtcFromAAAddress,
		model.Deposit{}.Column().B2TxStatus:       deposit.B2TxStatus,
		model.Deposit{}.Column().B2TxNonce:        deposit.B2TxNonce,
		model.Deposit{}.Column().B2TxFrom:         fromAddress,
	}
	err = bis.db.Model(&model.Deposit{}).Where("id = ?", deposit.ID).Updates(updateFields).Error
	if err != nil {
		return err
	}

	bis.log.Infow("invoke deposit send tx success, wait confirm",
		"data", deposit)

	// wait tx mined, may be wait long time so set timeout ctx
	ctx1, cancel1 := context.WithTimeout(context.Background(), WaitMinedTimeout)
	defer cancel1()
	errCh := make(chan error)
	go func(ctx1 context.Context, b2Tx *ethTypes.Transaction, deposit *model.Deposit) {
		err := bis.WaitMined(ctx1, b2Tx, deposit)
		if err != nil {
			errCh <- err
			return
		}
		//errCh <- nil
	}(ctx1, b2Tx, deposit)
	waitMinedTicker := time.NewTicker(WaitMinedTimeout + (10 * time.Second))
	bis.log.Warn("wait mined")
	select {
	case err = <-errCh:
		if err != nil {
			bis.log.Errorw("wait tx mined err", "error", err)
		}
		return err
	case <-bis.stopChan:
		bis.log.Errorw("wait tx mined stop chan", "error", err)
		cancel1()
		return ErrServerStop
	case <-waitMinedTicker.C:
		bis.log.Errorw("wait tx mined timeout", "error", err)
	}

	return nil
}

// HandleUnconfirmedDeposit
// 1. tx mined, update status
// 2. tx not mined, isPending, need reset gasprice
// 3. tx not mined, tx not mempool, need retry send tx
//
//nolint:dupl
func (bis *BridgeDepositService) HandleUnconfirmedDeposit(deposit *model.Deposit) error {
	resetNonce := false
	// 1. nonce to low, need reset nonce
	// 2. change from priv
	if deposit.B2TxStatus == model.DepositB2TxStatusNonceToLow ||
		!strings.EqualFold(deposit.B2TxFrom, bis.bridge.FromAddress()) {
		resetNonce = true
	}
	txReceipt, err := bis.bridge.TransactionReceipt(deposit.B2TxHash)
	if err == nil {
		// case 1
		updateFields := map[string]interface{}{}
		if txReceipt.Status == 1 {
			updateFields[model.Deposit{}.Column().B2TxStatus] = model.DepositB2TxStatusSuccess
		} else {
			updateFields[model.Deposit{}.Column().B2TxStatus] = model.DepositB2TxStatusWaitMinedStatusFailed
		}

		dbErr := bis.db.Model(&model.Deposit{}).Where("id = ?", deposit.ID).Updates(updateFields).Error
		if dbErr != nil {
			return dbErr
		}
		return err
	}
	bis.log.Errorw("TransactionReceipt err", "error", err, "data", deposit)
	if errors.Is(err, ethereum.NotFound) {
		bis.log.Errorf("TransactionReceipt not found")
		// tx in mempool, isPending
		tx, isPending, err := bis.bridge.TransactionByHash(deposit.B2TxHash)
		if err != nil {
			if errors.Is(err, ethereum.NotFound) || strings.Contains(err.Error(), "not found") {
				// case 3
				bis.log.Errorf("TransactionByHash not found, try send tx by nonce")
				return bis.HandleDeposit(deposit, nil, deposit.B2TxNonce, resetNonce)
			}
			return err
		}
		if isPending {
			// case 2
			bis.log.Warnw("tx is pending retry", "old", tx, "deposit", deposit)
			return bis.HandleDeposit(deposit, tx, 0, resetNonce)
		}
	}
	return err
}

func (bis *BridgeDepositService) WaitMined(ctx1 context.Context, b2Tx *ethTypes.Transaction, deposit *model.Deposit) error {
	b2txReceipt, err := bis.bridge.WaitMined(ctx1, b2Tx, nil)
	if err != nil {
		switch {
		case errors.Is(err, ErrBridgeWaitMinedStatus):
			deposit.B2TxStatus = model.DepositB2TxStatusWaitMinedStatusFailed
			bis.log.Errorw("invoke deposit wait mined err, status != 1",
				"btcTxHash", deposit.BtcTxHash,
				"b2txReceipt", b2txReceipt,
				"data", deposit)
		case errors.Is(err, context.DeadlineExceeded):
			// handle ctx deadline timeout
			// Indicates that the chain is unavailable at this time
			// This particular error needs to be recorded and handled manually
			deposit.B2TxStatus = model.DepositB2TxStatusContextDeadlineExceeded
			bis.log.Errorw("invoke deposit wait mined context deadline exceeded",
				"error", err.Error(),
				"btcTxHash", deposit.BtcTxHash,
				"data", deposit)
		default:
			deposit.B2TxStatus = model.DepositB2TxStatusWaitMinedFailed
			bis.log.Errorw("invoke deposit wait mined unknown err",
				"error", err.Error(),
				"btcTxHash", deposit.BtcTxHash,
				"data", deposit)
		}
	} else {
		deposit.B2TxStatus = model.DepositB2TxStatusSuccess
	}
	err = bis.db.Model(&model.Deposit{}).Where("id = ?", deposit.ID).Updates(map[string]interface{}{
		model.Deposit{}.Column().B2TxStatus: deposit.B2TxStatus,
	}).Error
	if err != nil {
		return err
	}
	if deposit.B2TxStatus == model.DepositB2TxStatusSuccess {
		bis.log.Infow("handle deposit success", "btcTxHash", deposit.BtcTxHash, "deposit", deposit)
	} else {
		bis.log.Errorw("handle deposit failed", "btcTxHash", deposit.BtcTxHash, "deposit", deposit)
		return fmt.Errorf("wait mined err b2_tx_status: %v", deposit.B2TxStatus)
	}
	return nil
}

func (bis *BridgeDepositService) CheckDeposit() {
	//ticker := time.NewTicker(BatchDepositWaitTimeout)
	defer func() {
		bis.wg.Done()
		//ticker.Stop()
	}()
	for {
		select {
		case <-bis.stopChan:
			bis.log.Warnf("check deposit stopping...")
			return
		case <-time.After(BatchDepositWaitTimeout):
			var deposits []model.Deposit
			err := bis.db.
				Where(
					fmt.Sprintf("%s.%s IN (?)", model.Deposit{}.TableName(), model.Deposit{}.Column().B2TxStatus),
					[]int{
						model.DepositB2TxStatusSuccess,
						model.DepositB2TxStatusTxHashExist,
					},
				).
				Where(
					fmt.Sprintf("%s.%s = ?", model.Deposit{}.TableName(), model.Deposit{}.Column().B2TxCheck),
					model.B2CheckStatusPending,
				).
				Limit(BatchDepositLimit).
				Order(fmt.Sprintf("%s.%s ASC", model.Deposit{}.TableName(), model.Deposit{}.Column().B2TxNonce)).
				Find(&deposits).
				Error
			if err != nil {
				bis.log.Errorw("failed find tx from db", "error", err)
				continue
			}

			for _, deposit := range deposits {

				var rollupDeposit model.RollupDeposit
				err = bis.db.
					Where(
						fmt.Sprintf("%s.%s = ?", model.RollupDeposit{}.TableName(), model.Deposit{}.Column().BtcTxHash),
						deposit.BtcTxHash,
					).
					First(&rollupDeposit).Error
				if err != nil {
					bis.log.Errorw("find rollup deposit error", "err", err, "deposit", deposit)
					continue
				}
				if deposit.B2TxStatus == model.DepositB2TxStatusSuccess {
					if strings.EqualFold(deposit.BtcFromAAAddress, rollupDeposit.BtcFromAAAddress) &&
						deposit.BtcValue == rollupDeposit.BtcValue {
						deposit.B2TxCheck = model.B2CheckStatusSuccess
					} else {
						deposit.B2TxCheck = model.B2CheckStatusFailed
					}
					err = bis.db.Model(&model.Deposit{}).Where("id = ?", deposit.ID).Updates(map[string]interface{}{
						model.Deposit{}.Column().B2TxCheck: deposit.B2TxCheck,
					}).Error
					if err != nil {
						bis.log.Errorw("update deposit error", "err", err)
					}
				} else if (deposit.B2TxStatus == model.DepositB2TxStatusTxHashExist) &&
					((deposit.B2TxHash == "") || (deposit.B2TxHash != rollupDeposit.B2TxHash)) {
					tx, _, err := bis.bridge.TransactionByHash(rollupDeposit.B2TxHash)
					if err != nil {
						bis.log.Errorw("get tx receipt error", "err", err)
						continue
					}
					// update tx info from rollup event
					err = bis.db.Model(&model.Deposit{}).Where("id = ?", deposit.ID).Updates(map[string]interface{}{
						model.Deposit{}.Column().B2TxCheck:        model.B2CheckStatusSuccess,
						model.Deposit{}.Column().B2TxHash:         rollupDeposit.B2TxHash,
						model.Deposit{}.Column().BtcFromAAAddress: rollupDeposit.BtcFromAAAddress,
						model.Deposit{}.Column().B2TxNonce:        tx.Nonce(),
						model.Deposit{}.Column().B2TxStatus:       model.DepositB2TxStatusSuccess,
						model.Deposit{}.Column().B2TxFrom:         rollupDeposit.B2TxFrom,
					}).Error
					if err != nil {
						bis.log.Errorw("update deposit error", "err", err)
					}
				}

				<-time.After(2 * time.Second)
			}
		}
	}
}
