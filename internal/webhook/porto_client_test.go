package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/frstrtr/mongotron/pkg/logger"
)

// Cross-language vectors: PortoAPI's receiver tests (tests/test_mongotron_webhook_receiver.py)
// assert the same hex values, so both sides agree on the v1 and v2 signature formats.
const (
	vecSecret = "test-secret"
	vecBody   = `{"txHash":"abc","amount":"1500000"}`
	vecTS     = "1700000000"
	vecV1     = "1b1c2dc46330b648b6cccf855eac258c3b61df2ac5063a45dbb0fa4c0a44dcb8"
	vecV2     = "fb74b1529628b4fe74f299bc94ea94f8ed83ef99ea43ebbafff1c5540c546505"
)

func testLogger() *logger.Logger {
	l := logger.NewDefault()
	return &l
}

func hmacHex(secret string, parts ...[]byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	for _, p := range parts {
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil))
}

type capturedRequest struct {
	path    string
	headers http.Header
	body    []byte
}

type recorder struct {
	mu       sync.Mutex
	requests []capturedRequest
	status   int32
	hits     int32
}

func newRecorder(status int) (*recorder, *httptest.Server) {
	rec := &recorder{status: int32(status)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		atomic.AddInt32(&rec.hits, 1)
		rec.mu.Lock()
		rec.requests = append(rec.requests, capturedRequest{path: r.URL.Path, headers: r.Header.Clone(), body: body})
		rec.mu.Unlock()
		w.WriteHeader(int(atomic.LoadInt32(&rec.status)))
	}))
	return rec, srv
}

func (r *recorder) last(t *testing.T) capturedRequest {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.requests) == 0 {
		t.Fatal("no request received")
	}
	return r.requests[len(r.requests)-1]
}

func TestSignVectors(t *testing.T) {
	if got := Sign(vecSecret, []byte(vecBody)); got != vecV1 {
		t.Fatalf("Sign = %s, want %s", got, vecV1)
	}
	if got := SignV2(vecSecret, vecTS, []byte(vecBody)); got != vecV2 {
		t.Fatalf("SignV2 = %s, want %s", got, vecV2)
	}
	if Sign("", []byte(vecBody)) != "" || SignV2("", vecTS, []byte(vecBody)) != "" {
		t.Fatal("empty secret must produce an empty signature")
	}
}

func TestSetSignatureHeaders(t *testing.T) {
	h := http.Header{}
	now := time.Unix(1700000000, 0)
	SetSignatureHeaders(h, vecSecret, []byte(vecBody), now)
	if h.Get(HeaderSignature) != vecV1 || h.Get(HeaderSignatureV2) != vecV2 || h.Get(HeaderTimestamp) != vecTS {
		t.Fatalf("unexpected headers: %v", h)
	}

	empty := http.Header{}
	SetSignatureHeaders(empty, "", []byte(vecBody), now)
	if len(empty) != 0 {
		t.Fatalf("no secret must set no signature headers, got %v", empty)
	}
}

func TestIsPortoWebhookURL(t *testing.T) {
	cases := map[string]bool{
		"http://oplex7020:8000/v1/webhooks/mongotron/transfer":      true,
		"http://192.168.100.3:43069/v1/webhooks/mongotron/transfer": true,
		"https://api.example.com/v1/webhooks/mongotron/operation":   true,
		"https://proxy.example.com/porto/v1/webhooks/mongotron/x":   true,
		"http://localhost:8000/v1/webhooks/mongotron/transfer?x=1":  true,
		"https://merchant.example.com/hooks/tron":                   false,
		"https://example.com/v1/webhooks/other/transfer":            false,
		"": false,
	}
	for u, want := range cases {
		if got := IsPortoWebhookURL(u); got != want {
			t.Errorf("IsPortoWebhookURL(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestSendTransferNotificationIsSigned(t *testing.T) {
	rec, srv := newRecorder(http.StatusOK)
	defer srv.Close()

	c := NewPortoAPIClient(srv.URL+"/", "", vecSecret, "tron-nile", testLogger())
	event := CreateTRXTransferEvent("aabbccddeeff00112233445566778899", 100, 1700000000000, true,
		"TSa6Hr3mmPsCkLcQFEjieEtSeW1ASY6UVV", "TRRPU387srsJJKiv8DfEsPnLBiWBcRMfCu", 1500000,
		"TRRPU387srsJJKiv8DfEsPnLBiWBcRMfCu", "sub_1", "tron-nile")

	before := time.Now().Unix()
	if err := c.SendTransferNotification(context.Background(), event); err != nil {
		t.Fatalf("SendTransferNotification: %v", err)
	}
	got := rec.last(t)

	if got.path != DefaultTransferPath {
		t.Errorf("path = %s, want %s", got.path, DefaultTransferPath)
	}
	if sig := got.headers.Get(HeaderSignature); sig != hmacHex(vecSecret, got.body) {
		t.Errorf("v1 signature %q does not match body", sig)
	}
	ts := got.headers.Get(HeaderTimestamp)
	tsN, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || tsN < before || tsN > time.Now().Unix() {
		t.Errorf("timestamp %q not current", ts)
	}
	if sig := got.headers.Get(HeaderSignatureV2); sig != hmacHex(vecSecret, []byte(ts), []byte("."), got.body) {
		t.Errorf("v2 signature %q does not match timestamp+body", sig)
	}
	if got.headers.Get("X-MongoTron-Event") != "trx_transfer" || got.headers.Get("X-Subscription-ID") != "sub_1" {
		t.Errorf("unexpected event headers: %v", got.headers)
	}

	var decoded TransferEvent
	if err := json.Unmarshal(got.body, &decoded); err != nil {
		t.Fatalf("body is not a TransferEvent: %v", err)
	}
	if decoded.Direction != "incoming" || decoded.Amount != "1500000" || decoded.AmountDecimal != "1.500000" {
		t.Errorf("unexpected event: %+v", decoded)
	}
}

func TestSendOperationNotificationUsesConfiguredPath(t *testing.T) {
	rec, srv := newRecorder(http.StatusOK)
	defer srv.Close()

	c := NewPortoAPIClient(srv.URL, "", vecSecret, "tron-nile", testLogger())
	if err := c.SendOperationNotification(context.Background(), &OperationEvent{
		EventType: "delegate_resource", OperationType: "DELEGATE", TxHash: "ff", SubscriptionID: "sub_2",
	}); err != nil {
		t.Fatalf("SendOperationNotification: %v", err)
	}
	if got := rec.last(t); got.path != DefaultOperationPath || got.headers.Get(HeaderSignature) != hmacHex(vecSecret, got.body) {
		t.Fatalf("default operation delivery wrong: path=%s headers=%v", got.path, got.headers)
	}

	c.SetOperationPath("custom/ops")
	if err := c.SendOperationNotification(context.Background(), &OperationEvent{OperationType: "VOTE", TxHash: "ee"}); err != nil {
		t.Fatalf("SendOperationNotification: %v", err)
	}
	if got := rec.last(t); got.path != "/custom/ops" {
		t.Fatalf("path = %s, want /custom/ops", got.path)
	}
}

func TestDeliveryDoesNotRetryClientErrors(t *testing.T) {
	rec, srv := newRecorder(http.StatusUnauthorized)
	defer srv.Close()

	c := NewPortoAPIClient(srv.URL, "", vecSecret, "tron-nile", testLogger())
	c.retryDelay = time.Millisecond
	err := c.SendTransferNotification(context.Background(), &TransferEvent{EventType: "trc20_transfer", TxHash: "aa"})
	if err == nil {
		t.Fatal("expected an error for a 401 response")
	}
	if hits := atomic.LoadInt32(&rec.hits); hits != 1 {
		t.Fatalf("401 was retried: %d attempts", hits)
	}
}

func TestDeliveryRetriesServerErrors(t *testing.T) {
	rec, srv := newRecorder(http.StatusServiceUnavailable)
	defer srv.Close()

	c := NewPortoAPIClient(srv.URL, "", vecSecret, "tron-nile", testLogger())
	c.retryDelay = time.Millisecond
	if err := c.SendTransferNotification(context.Background(), &TransferEvent{TxHash: "aa"}); err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	if hits := atomic.LoadInt32(&rec.hits); hits != maxDeliveryAttempts {
		t.Fatalf("attempts = %d, want %d", hits, maxDeliveryAttempts)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for i, r := range rec.requests {
		if r.headers.Get(HeaderSignature) == "" || r.headers.Get(HeaderSignatureV2) == "" {
			t.Fatalf("attempt %d was unsigned", i+1)
		}
	}
}

func TestEmptySecretSendsNoSignature(t *testing.T) {
	rec, srv := newRecorder(http.StatusOK)
	defer srv.Close()

	c := NewPortoAPIClient(srv.URL, "", "", "tron-nile", testLogger())
	if err := c.SendTransferNotification(context.Background(), &TransferEvent{TxHash: "aa"}); err != nil {
		t.Fatalf("SendTransferNotification: %v", err)
	}
	got := rec.last(t)
	for _, h := range []string{HeaderSignature, HeaderSignatureV2, HeaderTimestamp} {
		if _, ok := got.headers[http.CanonicalHeaderKey(h)]; ok {
			t.Errorf("header %s present without a secret", h)
		}
	}
}
