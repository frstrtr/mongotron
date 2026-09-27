package subscription

import (
	"testing"
	"time"

	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var resubBase = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func at(minutes int) time.Time { return resubBase.Add(time.Duration(minutes) * time.Minute) }

func timePtr(t time.Time) *time.Time { return &t }

// stoppedRow is a subscription that was stopped at stopMin after seeing lastSeen.
func stoppedRow(id string, createdMin, stopMin int, lastSeen int64, assets ...string) *models.Subscription {
	return &models.Subscription{
		SubscriptionID: id,
		Address:        "TRRPU387srsJJKiv8DfEsPnLBiWBcRMfCu",
		Status:         "stopped",
		Filters:        FiltersForAssets(assets, nil),
		WalletType:     "invoice",
		LastSeenBlock:  lastSeen,
		StoppedAt:      timePtr(at(stopMin)),
		StartBlock:     -1,
		CreatedAt:      at(createdMin),
	}
}

func activeRow(id string, createdMin int, startBlock int64, assets ...string) *models.Subscription {
	return &models.Subscription{
		SubscriptionID: id,
		Address:        "TRRPU387srsJJKiv8DfEsPnLBiWBcRMfCu",
		Status:         "active",
		Filters:        FiltersForAssets(assets, nil),
		WalletType:     "invoice",
		StartBlock:     startBlock,
		CurrentBlock:   startBlock,
		CreatedAt:      at(createdMin),
	}
}

func TestPlanResubscribe_ActiveIsRefreshedNotReactivated(t *testing.T) {
	// Regression: a stopped row with a last seen block used to be reactivated even
	// though the address already had an active subscription, giving two actives.
	stopped := stoppedRow("sub_old", 0, 10, 1000, "TRC20")
	active := activeRow("sub_new", 20, -1, "TRX", "TRC20")

	p := planResubscribe([]*models.Subscription{stopped, active}, ResubscribeOptions{})

	assert.Equal(t, ResubscribeRefreshed, p.action)
	require.NotNil(t, p.target)
	assert.Equal(t, "sub_new", p.target.SubscriptionID)
	assert.Empty(t, p.otherActive)
	assert.Equal(t, []string{"TRX", "TRC20"}, p.filters.AssetTypes)
	assert.ElementsMatch(t, []string{"TransferContract", "TriggerSmartContract"}, p.filters.ContractTypes)
	assert.Equal(t, "invoice", p.walletType)
}

func TestPlanResubscribe_ActiveReplacingStopRecoversItsGap(t *testing.T) {
	stopped := stoppedRow("sub_old", 0, 10, 1000, "TRC20")
	active := activeRow("sub_new", 20, -1, "TRC20")

	p := planResubscribe([]*models.Subscription{active, stopped}, ResubscribeOptions{ScanGap: true})

	require.NotNil(t, p.gapRow)
	assert.Equal(t, "sub_old", p.gapRow.SubscriptionID)
	assert.Equal(t, int64(1000), p.gapStart)
	assert.Equal(t, int64(0), p.gapEnd, "start not recorded: scan up to the current block")
}

func TestPlanResubscribe_ActiveWithRecordedStartEndsGapThere(t *testing.T) {
	stopped := stoppedRow("sub_old", 0, 10, 1000)
	active := activeRow("sub_new", 20, 1500)

	p := planResubscribe([]*models.Subscription{active, stopped}, ResubscribeOptions{})

	require.NotNil(t, p.gapRow)
	assert.Equal(t, int64(1000), p.gapStart)
	assert.Equal(t, int64(1500), p.gapEnd)
}

func TestPlanResubscribe_ActiveStartedBeforeStopHasNoGap(t *testing.T) {
	stopped := stoppedRow("sub_old", 0, 10, 1000)
	active := activeRow("sub_new", 20, 900)

	p := planResubscribe([]*models.Subscription{active, stopped}, ResubscribeOptions{})

	assert.Nil(t, p.gapRow)
	assert.Zero(t, p.gapStart)
	assert.Zero(t, p.gapEnd)
}

func TestPlanResubscribe_NoGapForActiveWhen(t *testing.T) {
	cases := map[string]func() []*models.Subscription{
		"gap already scanned": func() []*models.Subscription {
			stopped := stoppedRow("sub_old", 0, 10, 1000)
			stopped.GapScannedAt = timePtr(at(30))
			return []*models.Subscription{activeRow("sub_new", 20, -1), stopped}
		},
		"active row was reactivated": func() []*models.Subscription {
			active := activeRow("sub_new", 20, -1)
			active.StoppedAt = timePtr(at(25))
			active.LastSeenBlock = 1200
			return []*models.Subscription{active, stoppedRow("sub_old", 0, 10, 1000)}
		},
		"stopped while the active row ran": func() []*models.Subscription {
			return []*models.Subscription{activeRow("sub_new", 20, -1), stoppedRow("sub_old", 0, 30, 1000)}
		},
		"stop without last seen block": func() []*models.Subscription {
			return []*models.Subscription{activeRow("sub_new", 20, -1), stoppedRow("sub_old", 0, 10, 0)}
		},
		"only the active row": func() []*models.Subscription {
			return []*models.Subscription{activeRow("sub_new", 20, -1)}
		},
	}
	for name, rows := range cases {
		t.Run(name, func(t *testing.T) {
			p := planResubscribe(rows(), ResubscribeOptions{ScanGap: true})
			assert.Equal(t, ResubscribeRefreshed, p.action)
			assert.Equal(t, "sub_new", p.target.SubscriptionID)
			assert.Nil(t, p.gapRow)
		})
	}
}

func TestPlanResubscribe_LatestOfSeveralStopsDecides(t *testing.T) {
	// Older stop was followed by another subscription that was stopped too; only the
	// latest stop before the active row matters, and its gap was already scanned.
	older := stoppedRow("sub_1", 0, 10, 1000)
	newer := stoppedRow("sub_2", 20, 30, 2000)
	newer.GapScannedAt = timePtr(at(45))
	active := activeRow("sub_3", 40, -1)

	p := planResubscribe([]*models.Subscription{older, active, newer}, ResubscribeOptions{ScanGap: true})

	assert.Equal(t, "sub_3", p.target.SubscriptionID)
	assert.Nil(t, p.gapRow)
}

func TestPlanResubscribe_TwoActiveRefreshesNewestOnly(t *testing.T) {
	a := activeRow("sub_a", 0, -1)
	b := activeRow("sub_b", 10, -1)

	p := planResubscribe([]*models.Subscription{a, b}, ResubscribeOptions{})

	assert.Equal(t, ResubscribeRefreshed, p.action)
	assert.Equal(t, "sub_b", p.target.SubscriptionID)
	require.Len(t, p.otherActive, 1)
	assert.Equal(t, "sub_a", p.otherActive[0].SubscriptionID)
}

func TestPlanResubscribe_ReactivatesLatestRowWithItsFilters(t *testing.T) {
	older := stoppedRow("sub_old", 0, 10, 1000, "TRC20")
	latest := stoppedRow("sub_latest", 20, 30, 3000, "TRX", "TRC20")

	p := planResubscribe([]*models.Subscription{older, latest}, ResubscribeOptions{})

	assert.Equal(t, ResubscribeReactivated, p.action)
	assert.Equal(t, "sub_latest", p.target.SubscriptionID)
	assert.Equal(t, []string{"TRX", "TRC20"}, p.filters.AssetTypes)
	assert.ElementsMatch(t, []string{"TransferContract", "TriggerSmartContract"}, p.filters.ContractTypes,
		"TRX must stay watched: no TRC20-only filter on resubscribe")
	assert.True(t, p.filters.OnlySuccess)
	require.NotNil(t, p.gapRow)
	assert.Equal(t, int64(3000), p.gapStart)
	assert.Zero(t, p.gapEnd)
}

func TestPlanResubscribe_ReactivatesStopWithoutLastSeenBlock(t *testing.T) {
	stopped := stoppedRow("sub_old", 0, 10, 0, "TRX")

	p := planResubscribe([]*models.Subscription{stopped}, ResubscribeOptions{})

	assert.Equal(t, ResubscribeReactivated, p.action)
	assert.Equal(t, "sub_old", p.target.SubscriptionID)
	assert.Nil(t, p.gapRow)
}

func TestPlanResubscribe_AssetTypesOverride(t *testing.T) {
	stopped := stoppedRow("sub_old", 0, 10, 1000, "TRC20")
	stopped.Filters.TokenFilter = []string{"USDT"}
	stopped.Filters.MinAmount = 5

	p := planResubscribe([]*models.Subscription{stopped}, ResubscribeOptions{AssetTypes: []string{"TRX"}})

	assert.Equal(t, []string{"TRX"}, p.filters.AssetTypes)
	assert.Equal(t, []string{"TransferContract"}, p.filters.ContractTypes)
	assert.Equal(t, []string{"USDT"}, p.filters.TokenFilter, "token filter is kept")
	assert.Equal(t, int64(5), p.filters.MinAmount, "amount limits are kept")
	assert.True(t, p.filters.OnlySuccess)

	all := planResubscribe([]*models.Subscription{stopped}, ResubscribeOptions{AssetTypes: []string{}})
	assert.Equal(t, []string{}, all.filters.AssetTypes)
	assert.ElementsMatch(t, []string{"TransferContract", "TransferAssetContract", "TriggerSmartContract"}, all.filters.ContractTypes)
}

func TestPlanResubscribe_LegacyRowKeepsContractTypes(t *testing.T) {
	legacy := stoppedRow("sub_legacy", 0, 10, 1000)
	legacy.Filters = models.SubscriptionFilters{ContractTypes: []string{"TriggerSmartContract"}, OnlySuccess: true}

	p := planResubscribe([]*models.Subscription{legacy}, ResubscribeOptions{})

	assert.Equal(t, legacy.Filters, p.filters)
}

func TestPlanResubscribe_UnknownAddressCreates(t *testing.T) {
	p := planResubscribe(nil, ResubscribeOptions{})

	assert.Equal(t, ResubscribeCreated, p.action)
	assert.Nil(t, p.target)
	assert.Nil(t, p.gapRow)
	assert.Equal(t, "general", p.walletType)
	assert.Equal(t, FiltersForAssets(nil, nil), p.filters)

	withAssets := planResubscribe(nil, ResubscribeOptions{AssetTypes: []string{"TRC20"}, WalletType: "invoice"})
	assert.Equal(t, []string{"TriggerSmartContract"}, withAssets.filters.ContractTypes)
	assert.Equal(t, "invoice", withAssets.walletType)
}

func TestPlanResubscribe_WalletTypeOverride(t *testing.T) {
	p := planResubscribe([]*models.Subscription{activeRow("sub_a", 0, -1)}, ResubscribeOptions{WalletType: "platform"})
	assert.Equal(t, "platform", p.walletType)

	kept := planResubscribe([]*models.Subscription{activeRow("sub_a", 0, -1)}, ResubscribeOptions{})
	assert.Equal(t, "invoice", kept.walletType)
}

func TestWithAssetTypesNilKeepsFilters(t *testing.T) {
	stored := models.SubscriptionFilters{ContractTypes: []string{"TriggerSmartContract"}, TokenFilter: []string{"USDT"}}
	assert.Equal(t, stored, WithAssetTypes(stored, nil))
}
