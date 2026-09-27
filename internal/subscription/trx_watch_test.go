package subscription

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/frstrtr/mongotron/internal/blockchain/monitor"
	"github.com/frstrtr/mongotron/internal/blockchain/parser"
	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/frstrtr/mongotron/internal/webhook"
	"github.com/frstrtr/mongotron/pkg/logger"
)

// trxTransferEvent is the event the address monitor produces for a native TRX
// TransferContract(payer -> watched) of amountSun.
func trxTransferEvent(amountSun int64) *monitor.AddressEvent {
	return &monitor.AddressEvent{
		BlockNumber:    68900001,
		BlockTimestamp: 1758682966000,
		TransactionID:  "5f1c0d2b9a8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a3b2c1d0e9f8a7b6c5d4e3f2a",
		From:           parser.Base58ToHex(payerAddr),
		To:             parser.Base58ToHex(invoiceAdr),
		Amount:         amountSun,
		ContractType:   "TransferContract",
		Success:        true,
		EventData:      map[string]interface{}{},
	}
}

func TestFiltersForAssetsTRXAndTRC20(t *testing.T) {
	f := FiltersForAssets([]string{"TRX", "TRC20"}, nil)
	if len(f.ContractTypes) != 2 || !containsString(f.ContractTypes, "TransferContract") || !containsString(f.ContractTypes, "TriggerSmartContract") {
		t.Fatalf("contract types = %v", f.ContractTypes)
	}
	if !f.OnlySuccess {
		t.Error("watchlist filters must only pass successful transactions")
	}
	if all := FiltersForAssets(nil, nil); len(all.ContractTypes) != 3 {
		t.Errorf("empty asset list must mean all transfer types, got %v", all.ContractTypes)
	}
}

func TestTRXTransferToWatchedAddressProducesPortoEventInSun(t *testing.T) {
	m := &Manager{}
	event := trxTransferEvent(5_250_000) // 5.25 TRX

	// The TRC20-only filter was the incident: TRX to the address was dropped.
	if m.matchesFilters(event, FiltersForAssets([]string{"TRC20"}, nil)) {
		t.Fatal("a TRC20-only subscription must not pass a TransferContract")
	}
	filters := FiltersForAssets([]string{"TRX", "TRC20"}, nil)
	if !m.matchesFilters(event, filters) {
		t.Fatal("a TRX+TRC20 subscription must pass a TransferContract")
	}

	srv, ch := captureServer(t)
	r := newTestRouter(t)
	l := logger.NewDefault()
	r.SetPortoClient(webhook.NewPortoAPIClient(srv.URL, "", testSecret, "tron-nile", &l))
	r.SetNetwork("tron-nile")

	r.handleTRXTransfer(&RouteEventRequest{
		Subscription: &models.Subscription{
			SubscriptionID: "sub_trx",
			Address:        invoiceAdr,
			WalletType:     "invoice",
			UserID:         "customer_7",
			Filters:        filters,
			Metadata:       map[string]interface{}{"customer_ref": "c-1"},
		},
		Event: event,
	})
	got := waitFor(t, ch)

	if got.path != webhook.DefaultTransferPath {
		t.Errorf("path = %s", got.path)
	}
	if got.headers.Get(webhook.HeaderSignature) != hmacHex(testSecret, got.body) {
		t.Error("TRX transfer event is not validly signed")
	}

	var ev webhook.TransferEvent
	if err := json.Unmarshal(got.body, &ev); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ev.AssetType != "TRX" || ev.AssetID != "" || ev.EventType != "trx_transfer" || ev.Decimals != 6 {
		t.Errorf("unexpected asset fields: %+v", ev)
	}
	if ev.Amount != "5250000" || ev.AmountDecimal != "5.250000" {
		t.Errorf("amount = %q / %q, want 5250000 SUN / 5.250000", ev.Amount, ev.AmountDecimal)
	}
	if ev.Direction != "incoming" || ev.To != invoiceAdr || ev.From != payerAddr || ev.WatchedAddress != invoiceAdr {
		t.Errorf("unexpected classification: %+v", ev)
	}
	if ev.WalletType != "invoice" || ev.UserID != "customer_7" || ev.SubscriptionID != "sub_trx" || !ev.Success {
		t.Errorf("unexpected metadata: %+v", ev)
	}
}

func TestApplySettingsRefreshesRunningFilters(t *testing.T) {
	m := &Manager{}
	w := &MonitorWrapper{Subscription: &models.Subscription{
		SubscriptionID: "sub_live",
		Address:        invoiceAdr,
		WalletType:     "invoice",
		Filters:        FiltersForAssets([]string{"TRC20"}, nil),
		CurrentBlock:   100,
	}}
	before := w.current()
	event := trxTransferEvent(1_000_000)

	if m.matchesFilters(event, w.current().Filters) {
		t.Fatal("precondition: TRC20-only filter drops TRX")
	}

	w.applySettings(SubscriptionUpdate{
		Filters:    FiltersForAssets([]string{"TRX", "TRC20"}, nil),
		WalletType: "invoice",
		UserID:     "customer_7",
		Label:      "cust 1",
		Metadata:   map[string]interface{}{"assets": []string{"TRX", "TRC20"}},
	})

	now := w.current()
	if !m.matchesFilters(event, now.Filters) {
		t.Fatal("the running monitor must apply the new filters immediately")
	}
	if now.SubscriptionID != "sub_live" || now.CurrentBlock != 100 || now.UserID != "customer_7" || now.Label != "cust 1" {
		t.Errorf("unexpected snapshot after update: %+v", now)
	}
	// Snapshots handed to the router earlier are never modified.
	if containsString(before.Filters.ContractTypes, "TransferContract") || before.UserID != "" {
		t.Error("an earlier snapshot was mutated in place")
	}
}

func TestAdvanceCurrentBlockKeepsSettings(t *testing.T) {
	w := &MonitorWrapper{Subscription: &models.Subscription{
		SubscriptionID: "sub_blk",
		CurrentBlock:   10,
	}}
	w.applySettings(SubscriptionUpdate{Filters: FiltersForAssets([]string{"TRX"}, nil), Label: "x"})

	if !w.advanceCurrentBlock(20) || w.advanceCurrentBlock(15) {
		t.Fatal("block must only move forward")
	}
	cur := w.current()
	if cur.CurrentBlock != 20 || cur.Label != "x" || !containsString(cur.Filters.ContractTypes, "TransferContract") {
		t.Errorf("unexpected snapshot: %+v", cur)
	}
}

func TestWrapperSnapshotsAreRaceFree(t *testing.T) {
	// Run with -race: concurrent block progress, settings updates and readers.
	w := &MonitorWrapper{Subscription: &models.Subscription{SubscriptionID: "sub_race"}}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			for b := int64(0); b < 200; b++ {
				w.advanceCurrentBlock(b + int64(i))
			}
		}(i)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				w.applySettings(SubscriptionUpdate{Filters: FiltersForAssets([]string{"TRX", "TRC20"}, nil)})
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				sub := w.current()
				_ = sub.Filters.ContractTypes
				_ = sub.CurrentBlock
			}
		}()
	}
	wg.Wait()
	if w.current().CurrentBlock != 202 {
		t.Errorf("current block = %d, want 202", w.current().CurrentBlock)
	}
}
