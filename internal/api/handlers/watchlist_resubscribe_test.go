package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/frstrtr/mongotron/internal/subscription"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func doJSON(t *testing.T, app *fiber.App, method, path string, body []byte) (int, map[string]interface{}) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := app.Test(req)
	require.NoError(t, err)
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &out), string(raw))
	return resp.StatusCode, out
}

const resubscribePath = "/api/v1/watchlist/" + watchAddr + "/resubscribe"

func resubscribedSub(assets ...string) *models.Subscription {
	return &models.Subscription{
		SubscriptionID: "sub_a",
		Address:        watchAddr,
		Status:         "active",
		WalletType:     "invoice",
		Filters:        subscription.FiltersForAssets(assets, nil),
		CreatedAt:      time.Now(),
	}
}

func TestResubscribe_PassesStoredFiltersThroughWhenAssetTypesOmitted(t *testing.T) {
	m := new(MockSubscriptionManager)
	m.On("Resubscribe", subscription.ResubscribeOptions{
		Address:    watchAddr,
		WebhookURL: "https://example.invalid/hook",
		ScanGap:    true,
	}).Return(&subscription.ResubscribeResult{
		Subscription: resubscribedSub("TRX", "TRC20"),
		Action:       subscription.ResubscribeReactivated,
		GapDetected:  true,
		GapStart:     1000,
		GapEnd:       1600,
		GapScanning:  true,
	}, nil)

	status, out := doJSON(t, watchApp(m), "POST", resubscribePath,
		[]byte(`{"scanGap":true,"webhookUrl":"https://example.invalid/hook"}`))

	assert.Equal(t, fiber.StatusOK, status)
	m.AssertExpectations(t)
	// Existing fields keep their shape
	assert.Equal(t, "sub_a", out["subscriptionId"])
	assert.Equal(t, watchAddr, out["address"])
	assert.Equal(t, "active", out["status"])
	assert.Equal(t, true, out["gapDetected"])
	assert.Equal(t, float64(1000), out["gapStart"])
	assert.Equal(t, float64(1600), out["gapEnd"])
	assert.Equal(t, float64(600), out["gapBlocks"])
	assert.Equal(t, true, out["gapScanning"])
	assert.Contains(t, out["message"], "Background scan started")
	// Added fields
	assert.Equal(t, "reactivated", out["action"])
	assert.Equal(t, "invoice", out["walletType"])
	assert.Equal(t, []interface{}{"TRX", "TRC20"}, out["assetTypes"])
	filters, ok := out["filters"].(map[string]interface{})
	require.True(t, ok)
	assert.ElementsMatch(t, []interface{}{"TransferContract", "TriggerSmartContract"}, filters["contractTypes"])
}

func TestResubscribe_AssetTypesAndWalletTypeOverride(t *testing.T) {
	m := new(MockSubscriptionManager)
	m.On("Resubscribe", subscription.ResubscribeOptions{
		Address:    watchAddr,
		AssetTypes: []string{"TRX"},
		WalletType: "platform",
	}).Return(&subscription.ResubscribeResult{
		Subscription: resubscribedSub("TRX"),
		Action:       subscription.ResubscribeRefreshed,
	}, nil)

	status, out := doJSON(t, watchApp(m), "POST", resubscribePath,
		[]byte(`{"assetTypes":["TRX"],"walletType":"platform"}`))

	assert.Equal(t, fiber.StatusOK, status)
	m.AssertExpectations(t)
	assert.Equal(t, "refreshed", out["action"])
	assert.Equal(t, false, out["gapDetected"])
	assert.NotContains(t, out, "gapBlocks")
	assert.Contains(t, out["message"], "refreshed in place")
}

func TestResubscribe_EmptyAssetTypesMeansAllTypes(t *testing.T) {
	m := new(MockSubscriptionManager)
	m.On("Resubscribe", mock.MatchedBy(func(opts subscription.ResubscribeOptions) bool {
		return opts.AssetTypes != nil && len(opts.AssetTypes) == 0
	})).Return(&subscription.ResubscribeResult{
		Subscription: resubscribedSub(),
		Action:       subscription.ResubscribeRefreshed,
	}, nil)

	status, out := doJSON(t, watchApp(m), "POST", resubscribePath, []byte(`{"assetTypes":[]}`))

	assert.Equal(t, fiber.StatusOK, status)
	m.AssertExpectations(t)
	assert.Equal(t, []interface{}{}, out["assetTypes"], "assetTypes is always present")
}

func TestResubscribe_NoBodyDefaultsToGapScan(t *testing.T) {
	m := new(MockSubscriptionManager)
	m.On("Resubscribe", subscription.ResubscribeOptions{Address: watchAddr, ScanGap: true}).
		Return(&subscription.ResubscribeResult{
			Subscription: resubscribedSub("TRC20"),
			Action:       subscription.ResubscribeCreated,
		}, nil)

	status, out := doJSON(t, watchApp(m), "POST", resubscribePath, nil)

	assert.Equal(t, fiber.StatusOK, status)
	m.AssertExpectations(t)
	assert.Equal(t, "created", out["action"])
	assert.Equal(t, "New subscription created (no previous subscription found).", out["message"])
}

func TestResubscribe_InvalidWalletTypeRejected(t *testing.T) {
	m := new(MockSubscriptionManager)

	status, out := doJSON(t, watchApp(m), "POST", resubscribePath, []byte(`{"walletType":"bogus"}`))

	assert.Equal(t, fiber.StatusBadRequest, status)
	assert.Equal(t, "invalid_wallet_type", out["error"])
	m.AssertNotCalled(t, "Resubscribe", mock.Anything)
}

func TestResubscribe_ManagerErrorIs500(t *testing.T) {
	m := new(MockSubscriptionManager)
	m.On("Resubscribe", mock.Anything).Return(nil, fmt.Errorf("node down"))

	status, out := doJSON(t, watchApp(m), "POST", resubscribePath, []byte(`{"scanGap":true}`))

	assert.Equal(t, fiber.StatusInternalServerError, status)
	assert.Equal(t, "resubscribe_failed", out["error"])
}

func TestResubscribeMessage(t *testing.T) {
	cases := []struct {
		result subscription.ResubscribeResult
		want   string
	}{
		{subscription.ResubscribeResult{Action: subscription.ResubscribeReactivated}, "Resubscribed successfully. No gap recorded."},
		{subscription.ResubscribeResult{Action: subscription.ResubscribeReactivated, GapDetected: true}, "Resubscribed successfully. Gap detected but scan not requested."},
		{subscription.ResubscribeResult{Action: subscription.ResubscribeRefreshed, GapDetected: true, GapScanning: true}, "Already subscribed; settings refreshed in place. Background scan started to recover transactions missed before this subscription started."},
		{subscription.ResubscribeResult{Action: subscription.ResubscribeRefreshed}, "Already subscribed; settings refreshed in place. No gap to recover."},
	}
	for _, tc := range cases {
		r := tc.result
		assert.Equal(t, tc.want, resubscribeMessage(&r))
	}
}

const watchPath = "/api/v1/watchlist/" + watchAddr

func TestRemoveFromWatchList_ActiveThenStoppedIsIdempotent(t *testing.T) {
	fake := newFakeWatchManager()
	app := watchApp(fake)
	_, row, _ := postWatch(t, app, invoiceTRC20Body)

	status, out := doJSON(t, app, "DELETE", watchPath, nil)
	assert.Equal(t, fiber.StatusOK, status)
	assert.Equal(t, "Address removed from watch list", out["message"])
	assert.Equal(t, watchAddr, out["address"])
	assert.Equal(t, "stopped", out["status"])
	assert.Equal(t, row.SubscriptionID, out["subscriptionId"])
	assert.Equal(t, "stopped", fake.subs[row.SubscriptionID].Status)

	status, out = doJSON(t, app, "DELETE", watchPath, nil)
	assert.Equal(t, fiber.StatusOK, status, "removing a stopped address must not fail")
	assert.Equal(t, "Address already stopped", out["message"])
	assert.Equal(t, watchAddr, out["address"])
	assert.Equal(t, "stopped", out["status"])
	assert.Equal(t, 1, fake.unsubscribes, "no second unsubscribe for a stopped address")
}

func TestRemoveFromWatchList_UnknownAddressIs404(t *testing.T) {
	status, out := doJSON(t, watchApp(newFakeWatchManager()), "DELETE", watchPath, nil)

	assert.Equal(t, fiber.StatusNotFound, status)
	assert.Equal(t, "not_found", out["error"])
}

func TestRemoveFromWatchList_StopsTheActiveRowWhenAStoppedOneExists(t *testing.T) {
	fake := newFakeWatchManager()
	app := watchApp(fake)
	_, old, _ := postWatch(t, app, invoiceTRC20Body)
	fake.subs[old.SubscriptionID].Status = "stopped"
	_, current, _ := postWatch(t, app, customerTRXBody)
	require.NotEqual(t, old.SubscriptionID, current.SubscriptionID)

	status, out := doJSON(t, app, "DELETE", watchPath, nil)

	assert.Equal(t, fiber.StatusOK, status)
	assert.Equal(t, current.SubscriptionID, out["subscriptionId"])
	assert.Equal(t, "Address removed from watch list", out["message"])
	assert.Empty(t, fake.activeFor(watchAddr))
}

func TestRemoveFromWatchList_LookupFailureIs500(t *testing.T) {
	m := new(MockSubscriptionManager)
	m.On("GetByAddress", watchAddr).Return(nil, fmt.Errorf("storage down"))

	status, out := doJSON(t, watchApp(m), "DELETE", watchPath, nil)

	assert.Equal(t, fiber.StatusInternalServerError, status)
	assert.Equal(t, "lookup_failed", out["error"])
	m.AssertNotCalled(t, "Unsubscribe", mock.Anything)
}

func TestRemoveFromWatchList_UnsubscribeFailureIs500(t *testing.T) {
	m := new(MockSubscriptionManager)
	m.On("GetByAddress", watchAddr).Return(resubscribedSub("TRC20"), nil)
	m.On("Unsubscribe", "sub_a").Return(fmt.Errorf("storage down"))

	status, out := doJSON(t, watchApp(m), "DELETE", watchPath, nil)

	assert.Equal(t, fiber.StatusInternalServerError, status)
	assert.Equal(t, "unsubscribe_failed", out["error"])
}
