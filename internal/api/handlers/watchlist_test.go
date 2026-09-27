package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/frstrtr/mongotron/internal/subscription"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const watchAddr = "TRRPU387srsJJKiv8DfEsPnLBiWBcRMfCu"

// fakeWatchManager is a stateful in-memory manager: enough of the real one to
// check that repeated posts converge on one subscription per address.
type fakeWatchManager struct {
	MockSubscriptionManager // unused methods
	mu                      sync.Mutex
	subs                    map[string]*models.Subscription // key: subscription ID
	creates                 int
	updates                 int
}

func newFakeWatchManager() *fakeWatchManager {
	return &fakeWatchManager{subs: make(map[string]*models.Subscription)}
}

func (f *fakeWatchManager) SubscribeWithOptions(opts subscription.SubscribeOptions) (*models.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates++
	sub := &models.Subscription{
		SubscriptionID: fmt.Sprintf("sub_%d", f.creates),
		Address:        opts.Address,
		WebhookURL:     opts.WebhookURL,
		Filters:        opts.Filters,
		Status:         "active",
		StartBlock:     opts.StartBlock,
		CurrentBlock:   opts.StartBlock,
		WalletType:     opts.WalletType,
		UserID:         opts.UserID,
		Label:          opts.Label,
		Metadata:       opts.Metadata,
		CreatedAt:      time.Now(),
	}
	f.subs[sub.SubscriptionID] = sub
	cp := *sub
	return &cp, nil
}

func (f *fakeWatchManager) UpdateSubscription(id string, upd subscription.SubscriptionUpdate) (*models.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sub, ok := f.subs[id]
	if !ok || sub.Status != "active" {
		return nil, fmt.Errorf("active subscription not found")
	}
	f.updates++
	sub.Filters = upd.Filters
	sub.WalletType = upd.WalletType
	sub.UserID = upd.UserID
	sub.Label = upd.Label
	sub.Metadata = upd.Metadata
	cp := *sub
	return &cp, nil
}

func (f *fakeWatchManager) GetByAddress(address string) (*models.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var fallback *models.Subscription
	for _, sub := range f.subs {
		if sub.Address != address {
			continue
		}
		if sub.Status == "active" {
			cp := *sub
			return &cp, nil
		}
		fallback = sub
	}
	if fallback != nil {
		cp := *fallback
		return &cp, nil
	}
	return nil, fmt.Errorf("subscription not found for address: %s", address)
}

func (f *fakeWatchManager) List(limit, skip int64) ([]*models.Subscription, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*models.Subscription, 0, len(f.subs))
	for _, sub := range f.subs {
		cp := *sub
		out = append(out, &cp)
	}
	return out, int64(len(out)), nil
}

func (f *fakeWatchManager) activeFor(address string) []*models.Subscription {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*models.Subscription
	for _, sub := range f.subs {
		if sub.Address == address && sub.Status == "active" {
			out = append(out, sub)
		}
	}
	return out
}

func watchApp(m subscription.ManagerInterface) *fiber.App {
	app := fiber.New()
	h := NewWatchListHandler(m)
	app.Post("/api/v1/watchlist", h.AddToWatchList)
	app.Post("/api/v1/watchlist/bulk", h.BulkAddToWatchList)
	app.Get("/api/v1/watchlist", h.GetWatchList)
	app.Get("/api/v1/watchlist/:address", h.GetWatchedAddress)
	return app
}

func postWatch(t *testing.T, app *fiber.App, body string) (int, WatchListResponse, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/v1/watchlist", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	raw, _ := io.ReadAll(resp.Body)
	var row WatchListResponse
	var generic map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &row), string(raw))
	require.NoError(t, json.Unmarshal(raw, &generic), string(raw))
	return resp.StatusCode, row, generic
}

const invoiceTRC20Body = `{"address":"` + watchAddr + `","walletType":"invoice","userId":"customer_7","label":"cust 1","assetTypes":["TRC20"],"metadata":{"customer_ref":"c-1"}}`
const customerTRXBody = `{"address":"` + watchAddr + `","walletType":"invoice","userId":"customer_7","label":"cust 1 (TRX)","assetTypes":["TRX","TRC20"],"metadata":{"customer_ref":"c-1","assets":["TRX","TRC20"]}}`

func TestAddToWatchList_CreatesNewSubscription(t *testing.T) {
	fake := newFakeWatchManager()
	app := watchApp(fake)

	status, row, generic := postWatch(t, app, invoiceTRC20Body)

	assert.Equal(t, fiber.StatusCreated, status)
	assert.Equal(t, 1, fake.creates)
	assert.Equal(t, 0, fake.updates)
	assert.Equal(t, WalletTypeInvoice, row.WalletType)
	assert.Equal(t, "customer_7", row.UserID)
	assert.Equal(t, []string{"TRC20"}, row.AssetTypes)
	assert.Equal(t, []string{"TriggerSmartContract"}, row.Filters.ContractTypes)
	assert.True(t, row.Filters.OnlySuccess)
	assert.Contains(t, generic, "assetTypes")
	assert.Contains(t, generic, "filters")
}

func TestAddToWatchList_UpdatesFiltersInPlace(t *testing.T) {
	fake := newFakeWatchManager()
	app := watchApp(fake)

	status, first, _ := postWatch(t, app, invoiceTRC20Body)
	require.Equal(t, fiber.StatusCreated, status)

	status, second, _ := postWatch(t, app, customerTRXBody)

	assert.Equal(t, fiber.StatusOK, status)
	assert.Equal(t, first.SubscriptionID, second.SubscriptionID, "must update, not create")
	assert.Equal(t, 1, fake.creates)
	assert.Equal(t, 1, fake.updates)
	assert.Len(t, fake.activeFor(watchAddr), 1)

	assert.Equal(t, []string{"TRX", "TRC20"}, second.AssetTypes)
	assert.ElementsMatch(t, []string{"TransferContract", "TriggerSmartContract"}, second.Filters.ContractTypes)
	assert.True(t, second.Filters.OnlySuccess)
	assert.Equal(t, "cust 1 (TRX)", second.Label)
	assert.Equal(t, []interface{}{"TRX", "TRC20"}, second.Metadata["assets"])

	stored := fake.activeFor(watchAddr)[0]
	assert.Equal(t, []string{"TRX", "TRC20"}, stored.Filters.AssetTypes)
}

func TestAddToWatchList_RepeatIsIdempotent(t *testing.T) {
	fake := newFakeWatchManager()
	app := watchApp(fake)

	_, first, _ := postWatch(t, app, customerTRXBody)
	status2, second, _ := postWatch(t, app, customerTRXBody)
	status3, third, _ := postWatch(t, app, customerTRXBody)

	assert.Equal(t, fiber.StatusOK, status2)
	assert.Equal(t, fiber.StatusOK, status3)
	assert.Equal(t, first.SubscriptionID, second.SubscriptionID)
	assert.Equal(t, first.SubscriptionID, third.SubscriptionID)
	assert.Equal(t, 1, fake.creates)
	assert.Len(t, fake.activeFor(watchAddr), 1)
	assert.Equal(t, first.Filters, third.Filters)
	assert.Equal(t, first.Label, third.Label)
	assert.Equal(t, first.Metadata, third.Metadata)
	assert.Equal(t, first.WalletType, third.WalletType)
	assert.Equal(t, first.UserID, third.UserID)
}

func TestAddToWatchList_OmittedFieldsKeepStoredValues(t *testing.T) {
	fake := newFakeWatchManager()
	app := watchApp(fake)

	_, first, _ := postWatch(t, app, customerTRXBody)
	status, second, _ := postWatch(t, app, `{"address":"`+watchAddr+`"}`)

	assert.Equal(t, fiber.StatusOK, status)
	assert.Equal(t, first.SubscriptionID, second.SubscriptionID)
	assert.Equal(t, WalletTypeInvoice, second.WalletType, "omitted walletType must not reset to general")
	assert.Equal(t, first.UserID, second.UserID)
	assert.Equal(t, first.Label, second.Label)
	assert.Equal(t, first.Metadata, second.Metadata)
	assert.Equal(t, first.Filters, second.Filters, "omitted assetTypes must keep the filters")
}

func TestAddToWatchList_StoppedSubscriptionCreatesNew(t *testing.T) {
	fake := newFakeWatchManager()
	app := watchApp(fake)

	_, first, _ := postWatch(t, app, invoiceTRC20Body)
	fake.subs[first.SubscriptionID].Status = "stopped"

	status, second, _ := postWatch(t, app, customerTRXBody)

	assert.Equal(t, fiber.StatusCreated, status)
	assert.NotEqual(t, first.SubscriptionID, second.SubscriptionID)
	assert.Equal(t, 0, fake.updates)
}

func TestAddToWatchList_UpdateFailureIs500(t *testing.T) {
	m := new(MockSubscriptionManager)
	existing := &models.Subscription{SubscriptionID: "sub_a", Address: watchAddr, Status: "active", WalletType: "invoice"}
	m.On("GetByAddress", watchAddr).Return(existing, nil)
	m.On("UpdateSubscription", "sub_a", mock.AnythingOfType("subscription.SubscriptionUpdate")).
		Return(nil, fmt.Errorf("storage down"))

	status, _, generic := postWatch(t, watchApp(m), customerTRXBody)

	assert.Equal(t, fiber.StatusInternalServerError, status)
	assert.Equal(t, "subscription_failed", generic["error"])
	m.AssertNotCalled(t, "SubscribeWithOptions", mock.Anything)
}

func TestAddToWatchList_InvalidWalletTypeRejected(t *testing.T) {
	m := new(MockSubscriptionManager)
	status, _, generic := postWatch(t, watchApp(m), `{"address":"`+watchAddr+`","walletType":"bogus"}`)
	assert.Equal(t, fiber.StatusBadRequest, status)
	assert.Equal(t, "invalid_wallet_type", generic["error"])
}

func TestBulkAddToWatchList_UpsertsExisting(t *testing.T) {
	fake := newFakeWatchManager()
	app := watchApp(fake)
	_, first, _ := postWatch(t, app, invoiceTRC20Body)

	other := "TSa6Hr3mmPsCkLcQFEjieEtSeW1ASY6UVV"
	body := `{"addresses":[{"address":"` + watchAddr + `","assetTypes":["TRX","TRC20"]},{"address":"` + other + `","assetTypes":["TRC20"]}]}`
	req := httptest.NewRequest("POST", "/api/v1/watchlist/bulk", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	raw, _ := io.ReadAll(resp.Body)
	var out BulkWatchResponse
	require.NoError(t, json.Unmarshal(raw, &out))

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Equal(t, 2, out.Added)
	assert.Equal(t, 1, out.Created)
	assert.Equal(t, 1, out.Updated)
	assert.Len(t, fake.activeFor(watchAddr), 1)
	assert.Equal(t, first.SubscriptionID, out.Success[0].SubscriptionID)
	assert.Equal(t, []string{"TRX", "TRC20"}, out.Success[0].AssetTypes)
}

func TestGetWatchList_ExposesAssetTypesAndFilters(t *testing.T) {
	fake := newFakeWatchManager()
	app := watchApp(fake)
	postWatch(t, app, customerTRXBody)

	resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/watchlist", nil))
	require.NoError(t, err)
	raw, _ := io.ReadAll(resp.Body)
	var out struct {
		WatchList []map[string]interface{} `json:"watchList"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	require.Len(t, out.WatchList, 1)
	row := out.WatchList[0]
	assert.Equal(t, []interface{}{"TRX", "TRC20"}, row["assetTypes"])
	filters := row["filters"].(map[string]interface{})
	assert.ElementsMatch(t, []interface{}{"TransferContract", "TriggerSmartContract"}, filters["contractTypes"])

	resp, err = app.Test(httptest.NewRequest("GET", "/api/v1/watchlist/"+watchAddr, nil))
	require.NoError(t, err)
	raw, _ = io.ReadAll(resp.Body)
	var single WatchListResponse
	require.NoError(t, json.Unmarshal(raw, &single))
	assert.Equal(t, []string{"TRX", "TRC20"}, single.AssetTypes)
}

func TestWatchListResponse_AssetTypesAlwaysPresent(t *testing.T) {
	// A subscription created without assetTypes (all transfer types) still carries
	// the key, as an empty list, so clients can detect upsert support.
	row := toWatchListResponse(&models.Subscription{
		SubscriptionID: "sub_legacy",
		Address:        watchAddr,
		Status:         "active",
		Filters:        subscription.FiltersForAssets(nil, nil),
	})
	raw, err := json.Marshal(row)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"assetTypes":[]`)
	assert.Contains(t, string(raw), `"TransferContract"`)
	assert.Equal(t, WalletTypeGeneral, row.WalletType)
}
