package _interface

import (
	"context"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/qday-io/qday-abel-bridge-indexer/internal/model"
)

// BitcoinBridge defines the interface of custom bitcoin bridge.
type BitcoinBridge interface {
	// Deposit transfers amout to address
	Deposit(string, model.BitcoinFrom, string, int64, *types.Transaction, uint64, bool) (*types.Transaction, []byte, string, string, error)
	// Transfer amount to address
	Transfer(model.BitcoinFrom, int64, *types.Transaction, uint64, bool) (*types.Transaction, string, error)
	// WaitMined wait mined
	WaitMined(context.Context, *types.Transaction, []byte) (*types.Receipt, error)
	// TransactionReceipt
	TransactionReceipt(hash string) (*types.Receipt, error)
	// TransactionByHash
	TransactionByHash(hash string) (*types.Transaction, bool, error)
	FromAddress() string
}
