package monitor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
)

func realInfo() *core.TransactionInfo {
	return &core.TransactionInfo{
		Id:          make([]byte, 32),
		BlockNumber: 68_900_000,
		Result:      core.TransactionInfo_SUCESS,
		Receipt:     &core.ResourceReceipt{Result: core.Transaction_Result_SUCCESS},
	}
}

func TestTxInfoSucceeded(t *testing.T) {
	reverted := realInfo()
	reverted.Result = core.TransactionInfo_FAILED
	reverted.Receipt.Result = core.Transaction_Result_REVERT
	revertReceiptOnly := realInfo()
	revertReceiptOnly.Receipt.Result = core.Transaction_Result_REVERT
	outOfEnergy := realInfo()
	outOfEnergy.Receipt.Result = core.Transaction_Result_OUT_OF_ENERGY
	trxTransfer := realInfo() // plain TransferContract: receipt result DEFAULT
	trxTransfer.Receipt.Result = core.Transaction_Result_DEFAULT
	noBlock := realInfo()
	noBlock.BlockNumber = 0

	cases := []struct {
		name string
		info *core.TransactionInfo
		want bool
	}{
		{"nil reply", nil, false},
		// What the node answered for the ASCII-hex id MongoTron used to send: an EMPTY reply,
		// whose zero-value Result is SUCESS. It must not read as success.
		{"empty reply", &core.TransactionInfo{}, false},
		{"not in a block", noBlock, false},
		{"reverted", reverted, false},
		{"revert receipt", revertReceiptOnly, false},
		{"out of energy", outOfEnergy, false},
		{"trc20 success", realInfo(), true},
		{"trx transfer", trxTransfer, true},
	}
	for _, c := range cases {
		if got := TxInfoSucceeded(c.info); got != c.want {
			t.Errorf("%s: TxInfoSucceeded = %v, want %v", c.name, got, c.want)
		}
	}
}

type fakeFetcher struct {
	replies []*core.TransactionInfo
	errs    []error
	calls   int
}

func (f *fakeFetcher) GetTransactionInfoById(ctx context.Context, txID string) (*core.TransactionInfo, error) {
	i := f.calls
	f.calls++
	if i < len(f.errs) && f.errs[i] != nil {
		return nil, f.errs[i]
	}
	if i < len(f.replies) {
		return f.replies[i], nil
	}
	return nil, errors.New("no more replies")
}

func TestFetchTxInfoRetriesUntilTheNodeHasIt(t *testing.T) {
	f := &fakeFetcher{errs: []error{errors.New("not found"), errors.New("not found")},
		replies: []*core.TransactionInfo{nil, nil, realInfo()}}
	info, err := fetchTxInfo(context.Background(), f, "ab", 3, time.Millisecond, time.Second)
	if err != nil || info == nil || f.calls != 3 {
		t.Fatalf("got info=%v err=%v calls=%d, want the third reply", info, err, f.calls)
	}
}

func TestFetchTxInfoGivesUp(t *testing.T) {
	f := &fakeFetcher{errs: []error{errors.New("a"), errors.New("b"), errors.New("c"), errors.New("d")}}
	info, err := fetchTxInfo(context.Background(), f, "ab", 3, time.Millisecond, time.Second)
	if err == nil || info != nil || f.calls != 3 {
		t.Fatalf("got info=%v err=%v calls=%d, want an error after 3 calls", info, err, f.calls)
	}
}

func TestFetchTxInfoStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := &fakeFetcher{errs: []error{errors.New("a"), errors.New("b")}}
	if _, err := fetchTxInfo(ctx, f, "ab", 3, time.Hour, time.Second); err == nil || f.calls != 1 {
		t.Fatalf("err=%v calls=%d, want a cancellation error after 1 call", err, f.calls)
	}
}
