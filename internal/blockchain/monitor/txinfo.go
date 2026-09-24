package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
)

// txInfoFetcher is the part of the TRON client the monitors need to judge a transaction.
type txInfoFetcher interface {
	GetTransactionInfoById(ctx context.Context, txID string) (*core.TransactionInfo, error)
}

const (
	// txInfoAttempts/txInfoRetryDelay: the address monitor reads a block the node has just
	// applied, so its execution info is normally there at once; retry briefly before giving up.
	txInfoAttempts   = 3
	txInfoRetryDelay = time.Second
	txInfoTimeout    = 10 * time.Second
)

// fetchTxInfo asks the node for txID's execution info, up to attempts times (attempt n waits
// n*delay before it). It returns the last error when the info never arrived.
func fetchTxInfo(ctx context.Context, f txInfoFetcher, txID string, attempts int, delay, timeout time.Duration) (*core.TransactionInfo, error) {
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("transaction info for %s: %w (last error: %v)", txID, ctx.Err(), lastErr)
			case <-time.After(time.Duration(attempt-1) * delay):
			}
		}
		attemptCtx, cancel := context.WithTimeout(ctx, timeout)
		info, err := f.GetTransactionInfoById(attemptCtx, txID)
		cancel()
		if err == nil && info != nil {
			return info, nil
		}
		if err == nil {
			err = fmt.Errorf("transaction info for %s: empty reply", txID)
		}
		lastErr = err
	}
	return nil, lastErr
}

// TxInfoSucceeded reports whether a gettransactioninfobyid reply PROVES the transaction executed
// successfully: it must be a real reply (id and block set), the execution result SUCESS, and a
// smart-contract receipt, when there is one, SUCCESS (not REVERT, OUT_OF_ENERGY, ...). Anything
// else, including no reply at all, is not a success: a reverted TRC20 transfer carries exactly
// the same call data (to, amount) as a successful one.
func TxInfoSucceeded(info *core.TransactionInfo) bool {
	if info == nil || len(info.GetId()) == 0 || info.GetBlockNumber() <= 0 {
		return false
	}
	if info.GetResult() != core.TransactionInfo_SUCESS {
		return false
	}
	if receipt := info.GetReceipt(); receipt != nil {
		switch receipt.GetResult() {
		case core.Transaction_Result_DEFAULT, core.Transaction_Result_SUCCESS:
		default:
			return false
		}
	}
	return true
}
