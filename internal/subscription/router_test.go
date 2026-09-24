package subscription

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/frstrtr/mongotron/internal/blockchain/monitor"
	"github.com/frstrtr/mongotron/internal/blockchain/parser"
	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/frstrtr/mongotron/internal/webhook"
	"github.com/frstrtr/mongotron/pkg/logger"
)

const (
	testSecret = "router-test-secret"
	payerAddr  = "TSa6Hr3mmPsCkLcQFEjieEtSeW1ASY6UVV"
	invoiceAdr = "TRRPU387srsJJKiv8DfEsPnLBiWBcRMfCu"
)

func newTestRouter(t *testing.T) *EventRouter {
	t.Helper()
	l := logger.NewDefault()
	return NewEventRouter(nil, &l)
}

func hmacHex(secret string, parts ...[]byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	for _, p := range parts {
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil))
}

type captured struct {
	path    string
	headers http.Header
	body    []byte
}

// captureServer records every request and signals each one on the returned channel.
func captureServer(t *testing.T) (*httptest.Server, chan captured) {
	t.Helper()
	ch := make(chan captured, 8)
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		ch <- captured{path: r.URL.Path, headers: r.Header.Clone(), body: body}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, ch
}

func waitFor(t *testing.T, ch chan captured) captured {
	t.Helper()
	select {
	case c := <-ch:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for webhook delivery")
		return captured{}
	}
}

func TestShouldSendSubscriptionWebhook(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"", false},
		{"http://oplex7020:8000/v1/webhooks/mongotron/transfer", false},
		{"http://192.168.100.3:43069/v1/webhooks/mongotron/transfer", false},
		{"http://oplex7020:8000/v1/webhooks/mongotron/operation", false},
		{"https://merchant.example.com/tron/events", true},
	}
	for _, c := range cases {
		if got := shouldSendSubscriptionWebhook(&models.Subscription{WebhookURL: c.url}); got != c.want {
			t.Errorf("shouldSendSubscriptionWebhook(%q) = %v, want %v", c.url, got, c.want)
		}
	}
	if shouldSendSubscriptionWebhook(nil) {
		t.Error("nil subscription must not be delivered")
	}
}

func TestParseRawAmount(t *testing.T) {
	huge, _ := new(big.Int).SetString("123456789012345678901234567890", 10)
	cases := []struct {
		in   interface{}
		want string
		ok   bool
	}{
		{"1500000", "1500000", true},
		{"123456789012345678901234567890", "123456789012345678901234567890", true},
		{float64(1500000), "1500000", true},
		{int64(42), "42", true},
		{int(7), "7", true},
		{uint64(9), "9", true},
		{json.Number("250"), "250", true},
		{huge, huge.String(), true},
		{"not-a-number", "", false},
		{"-5", "", false},
		{nil, "", false},
		{true, "", false},
	}
	for _, c := range cases {
		got, ok := parseRawAmount(c.in)
		if ok != c.ok {
			t.Errorf("parseRawAmount(%v) ok = %v, want %v", c.in, ok, c.ok)
			continue
		}
		if ok && got.String() != c.want {
			t.Errorf("parseRawAmount(%v) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestSendToWebhookSignsWithSubscriptionSecret(t *testing.T) {
	srv, ch := captureServer(t)
	r := newTestRouter(t)
	r.SetSubscriptionSecret(testSecret)

	body := []byte(`{"TransactionID":"abc"}`)
	r.sendToWebhook(&models.Subscription{SubscriptionID: "sub_x", WebhookURL: srv.URL + "/hook"}, body)
	got := waitFor(t, ch)

	if got.headers.Get(webhook.HeaderSignature) != hmacHex(testSecret, body) {
		t.Errorf("v1 signature mismatch: %v", got.headers)
	}
	ts := got.headers.Get(webhook.HeaderTimestamp)
	if ts == "" || got.headers.Get(webhook.HeaderSignatureV2) != hmacHex(testSecret, []byte(ts), []byte("."), body) {
		t.Errorf("v2 signature mismatch: %v", got.headers)
	}
	if got.headers.Get("X-Subscription-ID") != "sub_x" || got.headers.Get("X-MongoTron-Event") != "address.transaction" {
		t.Errorf("unexpected headers: %v", got.headers)
	}
}

func TestSendToWebhookUnsignedWithoutSecret(t *testing.T) {
	srv, ch := captureServer(t)
	r := newTestRouter(t)

	r.sendToWebhook(&models.Subscription{SubscriptionID: "sub_y", WebhookURL: srv.URL}, []byte(`{}`))
	got := waitFor(t, ch)
	if got.headers.Get(webhook.HeaderSignature) != "" || got.headers.Get(webhook.HeaderSignatureV2) != "" {
		t.Errorf("unexpected signature headers without a secret: %v", got.headers)
	}
}

// usdtTransferRequest builds the event MongoTron's monitor produces for a USDT
// transfer(payer -> invoice) seen from the subscription watching watched.
func usdtTransferRequest(watched string) *RouteEventRequest {
	return &RouteEventRequest{
		Subscription: &models.Subscription{
			SubscriptionID: "sub_" + watched[:6],
			Address:        watched,
			WalletType:     "invoice",
			UserID:         "invoice_42",
			WebhookURL:     "http://192.168.100.3:43069/v1/webhooks/mongotron/transfer",
		},
		Event: &monitor.AddressEvent{
			BlockNumber:    68900000,
			BlockTimestamp: 1758682963000,
			TransactionID:  "e8db4a17365a17c750e4235cd9a8f43842d9ea814a0a710a948f22a1b531cf06",
			From:           parser.Base58ToHex(payerAddr),
			To:             parser.USDTNileHex,
			ContractType:   "TriggerSmartContract",
			Success:        true,
			EventData: map[string]interface{}{
				"smartContract": map[string]interface{}{
					"methodSignature": "a9059cbb",
					"parameters": map[string]interface{}{
						"to":     parser.Base58ToHex(invoiceAdr),
						"amount": "1500000",
					},
				},
			},
		},
	}
}

func TestHandleTRC20TransferSendsSignedEventWithRawAmount(t *testing.T) {
	srv, ch := captureServer(t)
	r := newTestRouter(t)
	l := logger.NewDefault()
	r.SetPortoClient(webhook.NewPortoAPIClient(srv.URL, "", testSecret, "tron-nile", &l))
	r.SetNetwork("tron-nile")

	r.handleTRC20Transfer(usdtTransferRequest(invoiceAdr))
	got := waitFor(t, ch)

	if got.path != webhook.DefaultTransferPath {
		t.Errorf("path = %s", got.path)
	}
	if got.headers.Get(webhook.HeaderSignature) != hmacHex(testSecret, got.body) {
		t.Error("transfer event is not validly signed")
	}

	var ev webhook.TransferEvent
	if err := json.Unmarshal(got.body, &ev); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ev.Amount != "1500000" || ev.AmountDecimal != "1.500000" {
		t.Errorf("amount = %q / %q, want 1500000 / 1.500000", ev.Amount, ev.AmountDecimal)
	}
	if ev.Direction != "incoming" || ev.To != invoiceAdr || ev.From != payerAddr || ev.WatchedAddress != invoiceAdr {
		t.Errorf("unexpected classification: %+v", ev)
	}
	if ev.WalletType != "invoice" || ev.UserID != "invoice_42" || ev.Network != "tron-nile" || !ev.Success {
		t.Errorf("unexpected metadata: %+v", ev)
	}
}

func TestHandleTRC20TransferMarksPayerSideOutgoing(t *testing.T) {
	srv, ch := captureServer(t)
	r := newTestRouter(t)
	l := logger.NewDefault()
	r.SetPortoClient(webhook.NewPortoAPIClient(srv.URL, "", testSecret, "tron-nile", &l))

	r.handleTRC20Transfer(usdtTransferRequest(payerAddr))
	got := waitFor(t, ch)

	var ev webhook.TransferEvent
	if err := json.Unmarshal(got.body, &ev); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ev.Direction != "outgoing" || ev.WatchedAddress != payerAddr {
		t.Errorf("payer-side event must be outgoing: %+v", ev)
	}
}
