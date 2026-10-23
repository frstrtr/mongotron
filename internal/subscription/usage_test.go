package subscription

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/frstrtr/mongotron/internal/blockchain/client"
	"github.com/frstrtr/mongotron/internal/blockchain/monitor"
	"github.com/frstrtr/mongotron/internal/blockchain/parser"
	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/frstrtr/mongotron/internal/webhook"
	"github.com/frstrtr/mongotron/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
)

const usageTxID = "3b1f4f0c2a9e8d7c6b5a49382716051f0e1d2c3b4a5968778695a4b3c2d1e0f9"

func keepLevelSub() *models.Subscription {
	return &models.Subscription{
		SubscriptionID: "sub_keep42",
		Address:        invoiceAdr,
		WalletType:     WalletTypeKeepLevel,
		UserID:         "keep-level:42",
		Label:          "keep-level order 42",
		Metadata:       map[string]interface{}{"keep_level_order_id": 42},
		Filters:        FiltersForAssets([]string{"TRC20"}, nil),
		WebhookURL:     "http://porto.example.internal/v1/webhooks/mongotron/transfer",
	}
}

func rawAddr(t *testing.T, base58 string) []byte {
	t.Helper()
	raw, err := hex.DecodeString(parser.Base58ToHex(base58))
	require.NoError(t, err)
	require.Len(t, raw, 21)
	return raw
}

func txInfo(result core.TransactionInfoCode, receipt *core.ResourceReceipt, fee int64) *core.TransactionInfo {
	id, _ := client.TxIDBytes(usageTxID)
	return &core.TransactionInfo{Id: id, BlockNumber: 68900002, Result: result, Receipt: receipt, Fee: fee}
}

// contractCallEvent is the monitor event of owner calling a contract (approve) with info.
func contractCallEvent(t *testing.T, owner string, info *core.TransactionInfo) *monitor.AddressEvent {
	t.Helper()
	data, _ := hex.DecodeString("095ea7b3" + "000000000000000000000041" + "11223344556677889900aabbccddeeff00112233" + "00000000000000000000000000000000000000000000000000000000000f4240")
	param, err := anypb.New(&core.TriggerSmartContract{
		OwnerAddress:    rawAddr(t, owner),
		ContractAddress: rawAddr(t, parser.USDTNile),
		Data:            data,
	})
	require.NoError(t, err)
	tx := &core.Transaction{RawData: &core.TransactionRaw{Contract: []*core.Transaction_Contract{
		{Type: core.Transaction_Contract_TriggerSmartContract, Parameter: param},
	}}}
	return &monitor.AddressEvent{
		BlockNumber:    68900002,
		BlockTimestamp: 1758682969000,
		TransactionID:  usageTxID,
		From:           parser.Base58ToHex(owner),
		To:             parser.USDTNileHex,
		ContractType:   "TriggerSmartContract",
		Success:        monitor.TxInfoSucceeded(info),
		RawTransaction: tx,
		RawTxInfo:      info,
		EventData: map[string]interface{}{
			"smartContract": map[string]interface{}{"methodSignature": "095ea7b3", "methodName": "approve"},
		},
	}
}

func trxTransferRaw(t *testing.T, from, to string) *core.Transaction {
	t.Helper()
	param, err := anypb.New(&core.TransferContract{OwnerAddress: rawAddr(t, from), ToAddress: rawAddr(t, to), Amount: 1_000_000})
	require.NoError(t, err)
	return &core.Transaction{RawData: &core.TransactionRaw{Contract: []*core.Transaction_Contract{
		{Type: core.Transaction_Contract_TransferContract, Parameter: param},
	}}}
}

func TestBuildUsageEventForAContractCall(t *testing.T) {
	info := txInfo(core.TransactionInfo_SUCESS, &core.ResourceReceipt{
		EnergyUsage: 99_604, EnergyUsageTotal: 99_604, NetUsage: 313, Result: core.Transaction_Result_SUCCESS,
	}, 0)
	ev, ok := BuildUsageEvent(keepLevelSub(), contractCallEvent(t, invoiceAdr, info), "tron-nile")
	require.True(t, ok)

	assert.Equal(t, webhook.EventTypeEnergyUsage, ev.EventType)
	assert.Equal(t, "usage_"+usageTxID+"_sub_keep42", ev.EventID)
	assert.Equal(t, "tron-nile", ev.Network)
	assert.Equal(t, usageTxID, ev.TxHash)
	assert.Equal(t, int64(68900002), ev.BlockNumber)
	assert.Equal(t, int64(1758682969000), ev.BlockTimestamp)
	assert.True(t, ev.Success)
	assert.True(t, ev.ReceiptKnown)
	assert.Equal(t, "SUCCESS", ev.Result)
	assert.Equal(t, invoiceAdr, ev.OwnerAddress)
	assert.Equal(t, "TriggerSmartContract", ev.ContractType)
	assert.Equal(t, parser.USDTNile, ev.ContractAddress)
	assert.Equal(t, "095ea7b3", ev.MethodSignature)
	assert.Equal(t, "approve", ev.MethodName)
	assert.Equal(t, int64(99_604), ev.EnergyUsage)
	assert.Equal(t, int64(99_604), ev.EnergyUsageTotal)
	assert.Equal(t, int64(313), ev.NetUsage)
	assert.Equal(t, "outgoing", ev.Direction)
	assert.Equal(t, WalletTypeKeepLevel, ev.WalletType)
	assert.Equal(t, invoiceAdr, ev.WatchedAddress)
	assert.Equal(t, "sub_keep42", ev.SubscriptionID)
	assert.Equal(t, "keep-level:42", ev.UserID)
	assert.Equal(t, 42, ev.Metadata["keep_level_order_id"])

	pretty, _ := json.MarshalIndent(ev, "", "  ")
	t.Logf("energy_usage payload:\n%s", pretty)
}

func TestBuildUsageEventForAFailedCall(t *testing.T) {
	info := txInfo(core.TransactionInfo_FAILED, &core.ResourceReceipt{
		EnergyUsage: 40_000, EnergyUsageTotal: 150_000, EnergyFee: 22_000_000, NetUsage: 345,
		Result: core.Transaction_Result_OUT_OF_ENERGY,
	}, 22_000_000)
	info.ResMessage = []byte("Not enough energy for 'SSTORE' operation executing\n")

	ev, ok := BuildUsageEvent(keepLevelSub(), contractCallEvent(t, invoiceAdr, info), "tron-nile")
	require.True(t, ok, "a failed call consumed energy and must be reported")
	assert.False(t, ev.Success)
	assert.Equal(t, "OUT_OF_ENERGY", ev.Result)
	assert.Equal(t, "Not enough energy for 'SSTORE' operation executing", ev.ResultMessage)
	assert.Equal(t, int64(40_000), ev.EnergyUsage)
	assert.Equal(t, int64(150_000), ev.EnergyUsageTotal)
	assert.Equal(t, int64(22_000_000), ev.EnergyFee)
	assert.Equal(t, int64(22_000_000), ev.Fee)

	reverted := txInfo(core.TransactionInfo_FAILED, &core.ResourceReceipt{EnergyUsage: 1_200, Result: core.Transaction_Result_REVERT}, 0)
	ev, ok = BuildUsageEvent(keepLevelSub(), contractCallEvent(t, invoiceAdr, reverted), "tron-nile")
	require.True(t, ok)
	assert.Equal(t, "REVERT", ev.Result)
	assert.False(t, ev.Success)
}

func TestBuildUsageEventOnlyForTheOwner(t *testing.T) {
	info := txInfo(core.TransactionInfo_SUCESS, &core.ResourceReceipt{NetUsage: 268}, 0)

	// TRX received by the watched address: the sender paid, not the watched address
	incoming := &monitor.AddressEvent{TransactionID: usageTxID, ContractType: "TransferContract",
		RawTransaction: trxTransferRaw(t, payerAddr, invoiceAdr), RawTxInfo: info}
	_, ok := BuildUsageEvent(keepLevelSub(), incoming, "tron-nile")
	assert.False(t, ok)

	// A contract call by someone else that only names the watched address
	_, ok = BuildUsageEvent(keepLevelSub(), contractCallEvent(t, payerAddr, info), "tron-nile")
	assert.False(t, ok)

	// The watched address sending TRX: bandwidth only, still reported
	outgoing := &monitor.AddressEvent{TransactionID: usageTxID, ContractType: "TransferContract",
		RawTransaction: trxTransferRaw(t, invoiceAdr, payerAddr), RawTxInfo: info}
	ev, ok := BuildUsageEvent(keepLevelSub(), outgoing, "tron-nile")
	require.True(t, ok)
	assert.Equal(t, "TransferContract", ev.ContractType)
	assert.Empty(t, ev.ContractAddress)
	assert.Equal(t, int64(0), ev.EnergyUsage)
	assert.Equal(t, int64(268), ev.NetUsage)
	assert.Equal(t, "SUCCESS", ev.Result)
}

func TestBuildUsageEventWithoutTransactionInfo(t *testing.T) {
	ev, ok := BuildUsageEvent(keepLevelSub(), contractCallEvent(t, invoiceAdr, nil), "tron-nile")
	require.True(t, ok, "without info the event is still a hint")
	assert.False(t, ev.ReceiptKnown)
	assert.False(t, ev.Success)
	assert.Equal(t, "UNKNOWN", ev.Result)
	assert.Equal(t, int64(0), ev.EnergyUsage)
}

func TestBuildUsageEventSkipsWhenNothingWasConsumed(t *testing.T) {
	info := txInfo(core.TransactionInfo_SUCESS, &core.ResourceReceipt{}, 0)
	_, ok := BuildUsageEvent(keepLevelSub(), contractCallEvent(t, invoiceAdr, info), "tron-nile")
	assert.False(t, ok)
	_, ok = BuildUsageEvent(keepLevelSub(), &monitor.AddressEvent{TransactionID: usageTxID}, "tron-nile")
	assert.False(t, ok, "no raw transaction: owner unknown")
}

func usageRouter(t *testing.T, url string, walletTypes ...string) *EventRouter {
	t.Helper()
	r := newTestRouter(t)
	l := logger.NewDefault()
	r.SetPortoClient(webhook.NewPortoAPIClient(url, "", testSecret, "tron-nile", &l))
	r.SetNetwork("tron-nile")
	if len(walletTypes) > 0 {
		r.SetUsageEvents(walletTypes)
	}
	return r
}

func successfulCall(t *testing.T) *monitor.AddressEvent {
	info := txInfo(core.TransactionInfo_SUCESS, &core.ResourceReceipt{EnergyUsage: 64_285, EnergyUsageTotal: 64_285, NetUsage: 345, Result: core.Transaction_Result_SUCCESS}, 0)
	return contractCallEvent(t, invoiceAdr, info)
}

func TestRouteUsageDeliversASignedEnergyUsageEvent(t *testing.T) {
	srv, ch := captureServer(t)
	r := usageRouter(t, srv.URL, WalletTypeKeepLevel)

	require.True(t, r.RouteUsage(keepLevelSub(), successfulCall(t)))
	got := waitFor(t, ch)

	assert.Equal(t, webhook.DefaultUsagePath, got.path)
	assert.Equal(t, hmacHex(testSecret, got.body), got.headers.Get(webhook.HeaderSignature))
	ts := got.headers.Get(webhook.HeaderTimestamp)
	assert.Equal(t, hmacHex(testSecret, []byte(ts), []byte("."), got.body), got.headers.Get(webhook.HeaderSignatureV2))
	assert.Equal(t, "energy_usage", got.headers.Get("X-MongoTron-Event"))
	assert.Equal(t, "sub_keep42", got.headers.Get("X-Subscription-ID"))

	var ev webhook.UsageEvent
	require.NoError(t, json.Unmarshal(got.body, &ev))
	assert.Equal(t, int64(64_285), ev.EnergyUsage)
	assert.Equal(t, invoiceAdr, ev.OwnerAddress)
	assert.Equal(t, float64(42), ev.Metadata["keep_level_order_id"])
	waitUntil(t, func() bool { return r.UsageStats().Sent == 1 })
}

func TestRouteUsageOnlyForEnabledWalletTypes(t *testing.T) {
	srv, _ := captureServer(t)
	invoice := keepLevelSub()
	invoice.WalletType = "invoice"

	off := usageRouter(t, srv.URL)
	assert.False(t, off.RouteUsage(keepLevelSub(), successfulCall(t)), "usage events are off until enabled")

	keepOnly := usageRouter(t, srv.URL, WalletTypeKeepLevel)
	assert.False(t, keepOnly.RouteUsage(invoice, successfulCall(t)))
	assert.True(t, keepOnly.RouteUsage(keepLevelSub(), successfulCall(t)))

	all := usageRouter(t, srv.URL, "*")
	assert.True(t, all.RouteUsage(invoice, successfulCall(t)))
	assert.ElementsMatch(t, []string{"*"}, all.UsageWalletTypes())

	noPorto := newTestRouter(t)
	noPorto.SetUsageEvents([]string{WalletTypeKeepLevel})
	assert.False(t, noPorto.RouteUsage(keepLevelSub(), successfulCall(t)), "no Porto client: nothing to send")

	keepOnly.SetUsageEvents(nil)
	assert.False(t, keepOnly.RouteUsage(keepLevelSub(), successfulCall(t)), "an empty list disables the events")
}

func TestRouteUsageNeverBlocksOnASlowPorto(t *testing.T) {
	gate := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		<-gate
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(gate) })

	r := usageRouter(t, srv.URL)
	r.setUsageEvents([]string{WalletTypeKeepLevel}, 1)

	start := time.Now()
	for i := 0; i < usageWorkers+10; i++ {
		r.RouteUsage(keepLevelSub(), successfulCall(t))
	}
	assert.Less(t, time.Since(start), time.Second, "RouteUsage must not wait for delivery")
	assert.Greater(t, r.UsageStats().Dropped, int64(0), "a full queue drops instead of blocking")
}

func TestUsageIsRoutedEvenWhenTheFiltersDropTheEvent(t *testing.T) {
	srv, ch := captureServer(t)
	l := logger.NewDefault()
	m := newManager(nil, nil, &l, DefaultManagerOptions())
	m.eventRouter = usageRouter(t, srv.URL, WalletTypeKeepLevel)

	failed := contractCallEvent(t, invoiceAdr, txInfo(core.TransactionInfo_FAILED,
		&core.ResourceReceipt{EnergyUsage: 30_000, EnergyUsageTotal: 30_000, Result: core.Transaction_Result_REVERT}, 0))
	// The watchlist filters pass successful transactions only
	routed, err := m.routeMonitorEvent(keepLevelSub(), failed)
	assert.False(t, routed)
	assert.NoError(t, err)

	got := waitFor(t, ch)
	assert.Equal(t, webhook.DefaultUsagePath, got.path)
	var ev webhook.UsageEvent
	require.NoError(t, json.Unmarshal(got.body, &ev))
	assert.False(t, ev.Success)
	assert.Equal(t, "REVERT", ev.Result)
	assert.Equal(t, int64(30_000), ev.EnergyUsage)
}

func TestKeepLevelAddressesGetNoTransferEvents(t *testing.T) {
	srv, ch := captureServer(t)
	r := usageRouter(t, srv.URL, WalletTypeKeepLevel)

	req := usdtTransferRequest(invoiceAdr) // USDT received by the watched address
	req.Subscription.WalletType = WalletTypeKeepLevel
	r.routeEvent(req)
	select {
	case got := <-ch:
		t.Fatalf("a keep_level address must not produce Porto %s events", got.path)
	case <-time.After(300 * time.Millisecond):
	}

	invoice := usdtTransferRequest(invoiceAdr)
	r.routeEvent(invoice)
	got := waitFor(t, ch)
	assert.Equal(t, webhook.DefaultTransferPath, got.path, "other wallet types keep their transfer events")
}

// emptySource is a node with empty blocks.
type emptySource struct{}

func (emptySource) GetNowBlock(ctx context.Context) (*core.Block, error) {
	return &core.Block{BlockHeader: &core.BlockHeader{RawData: &core.BlockHeaderRaw{Number: 5000}}}, nil
}
func (emptySource) GetBlockByNum(ctx context.Context, n int64) (*core.Block, error) {
	return &core.Block{BlockHeader: &core.BlockHeader{RawData: &core.BlockHeaderRaw{Number: n}}}, nil
}
func (emptySource) GetTransactionInfoById(ctx context.Context, id string) (*core.TransactionInfo, error) {
	return nil, client.ErrTransactionInfoNotFound
}

func TestManagerRunsEveryAddressMonitorOnOneScanner(t *testing.T) {
	l := logger.NewDefault()
	m := newManager(nil, emptySource{}, &l, DefaultManagerOptions())
	for i, addr := range []string{invoiceAdr, payerAddr} {
		require.NoError(t, m.startMonitor(&models.Subscription{
			SubscriptionID: "sub_" + string(rune('a'+i)), Address: addr, Status: "active", CurrentBlock: 4990,
		}))
	}
	st, ok := m.ScannerStats()
	require.True(t, ok)
	assert.Equal(t, 2, st.Subscribers, "both monitors read from the shared scanner")
	require.NoError(t, m.Stop())
	st, _ = m.ScannerStats()
	assert.Equal(t, 0, st.Subscribers)

	polling := newManager(nil, emptySource{}, &l, ManagerOptions{SharedScanner: false})
	_, ok = polling.ScannerStats()
	assert.False(t, ok, "sharedScanner off: one polling loop per address")
	require.NoError(t, polling.Stop())
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
