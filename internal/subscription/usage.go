package subscription

import (
	"context"
	"encoding/hex"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/frstrtr/mongotron/internal/blockchain/monitor"
	"github.com/frstrtr/mongotron/internal/blockchain/parser"
	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/frstrtr/mongotron/internal/webhook"
	"google.golang.org/protobuf/proto"
)

const (
	// WalletTypeKeepLevel is the wallet type of an address watched only for its resource
	// usage (a keep-level energy rental receiver). It produces energy_usage events and no
	// transfer or operation events for Porto, so watching it can never credit anything.
	WalletTypeKeepLevel = "keep_level"

	// DefaultUsageWalletType is the wallet type energy_usage events are sent for when the
	// configuration does not list any.
	DefaultUsageWalletType = WalletTypeKeepLevel

	usageQueueSize   = 1000
	usageWorkers     = 4
	usageSendTimeout = 30 * time.Second
	maxResultMessage = 256
)

// usageRouting holds the energy_usage settings and the delivery queue of a router.
type usageRouting struct {
	mu      sync.RWMutex
	all     bool
	types   map[string]bool
	queue   chan *webhook.UsageEvent
	queued  atomic.Int64
	sent    atomic.Int64
	failed  atomic.Int64
	dropped atomic.Int64
}

// UsageStats are the energy_usage delivery counters of a router.
type UsageStats struct {
	Queued  int64 `json:"queued"`
	Sent    int64 `json:"sent"`
	Failed  int64 `json:"failed"`
	Dropped int64 `json:"dropped"`
}

// SetUsageEvents enables energy_usage events for watched addresses of the given wallet
// types ("*" means every wallet type). An empty list disables them. Delivery runs on a
// few background workers fed by a bounded queue: block processing never waits for it.
func (r *EventRouter) SetUsageEvents(walletTypes []string) {
	r.setUsageEvents(walletTypes, usageQueueSize)
}

func (r *EventRouter) setUsageEvents(walletTypes []string, queueSize int) {
	types := make(map[string]bool)
	all := false
	for _, t := range walletTypes {
		t = strings.TrimSpace(t)
		switch t {
		case "":
		case "*":
			all = true
		default:
			types[t] = true
		}
	}

	r.usage.mu.Lock()
	defer r.usage.mu.Unlock()
	r.usage.all = all
	r.usage.types = types
	if (all || len(types) > 0) && r.usage.queue == nil {
		r.usage.queue = make(chan *webhook.UsageEvent, queueSize)
		for i := 0; i < usageWorkers; i++ {
			go r.usageWorker(r.usage.queue)
		}
	}
}

// UsageWalletTypes returns the wallet types energy_usage events are sent for.
func (r *EventRouter) UsageWalletTypes() []string {
	r.usage.mu.RLock()
	defer r.usage.mu.RUnlock()
	out := make([]string, 0, len(r.usage.types)+1)
	if r.usage.all {
		out = append(out, "*")
	}
	for t := range r.usage.types {
		out = append(out, t)
	}
	return out
}

// UsageStats returns the energy_usage delivery counters.
func (r *EventRouter) UsageStats() UsageStats {
	return UsageStats{
		Queued:  r.usage.queued.Load(),
		Sent:    r.usage.sent.Load(),
		Failed:  r.usage.failed.Load(),
		Dropped: r.usage.dropped.Load(),
	}
}

func (r *EventRouter) usageEnabledFor(walletType string) (chan *webhook.UsageEvent, bool) {
	r.usage.mu.RLock()
	defer r.usage.mu.RUnlock()
	if r.usage.queue == nil {
		return nil, false
	}
	return r.usage.queue, r.usage.all || r.usage.types[walletType]
}

// RouteUsage queues an energy_usage event for Porto when the subscription's wallet type
// is enabled and the watched address is the transaction's owner. It does not depend on
// the subscription's transfer filters: a failed call, or a call of any contract, still
// consumed the owner's resources. It never blocks: when the queue is full the event is
// dropped (counted and logged). It reports whether an event was queued.
func (r *EventRouter) RouteUsage(sub *models.Subscription, event *monitor.AddressEvent) bool {
	if sub == nil || event == nil {
		return false
	}
	queue, enabled := r.usageEnabledFor(sub.WalletType)
	if !enabled || r.porto() == nil {
		return false
	}
	ev, ok := BuildUsageEvent(sub, event, r.networkName())
	if !ok {
		return false
	}
	select {
	case queue <- ev:
		r.usage.queued.Add(1)
		return true
	default:
		r.usage.dropped.Add(1)
		r.logger.Warn().
			Str("txHash", ev.TxHash).
			Str("subscriptionId", ev.SubscriptionID).
			Msg("Usage event queue full, dropping energy_usage event")
		return false
	}
}

func (r *EventRouter) usageWorker(queue <-chan *webhook.UsageEvent) {
	for ev := range queue {
		client := r.porto()
		if client == nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), usageSendTimeout)
		err := client.SendUsageNotification(ctx, ev)
		cancel()
		if err != nil {
			r.usage.failed.Add(1)
			r.logger.Error().
				Err(err).
				Str("txHash", ev.TxHash).
				Str("subscriptionId", ev.SubscriptionID).
				Msg("Failed to send energy_usage notification to Porto API")
			continue
		}
		r.usage.sent.Add(1)
	}
}

// BuildUsageEvent builds the energy_usage event of a monitor event for a subscription.
// It reports false when the watched address is not the transaction's owner, or when
// the transaction info shows that nothing was consumed.
func BuildUsageEvent(sub *models.Subscription, event *monitor.AddressEvent, network string) (*webhook.UsageEvent, bool) {
	if sub == nil || event == nil || event.RawTransaction == nil || event.RawTransaction.GetRawData() == nil {
		return nil, false
	}
	contracts := event.RawTransaction.GetRawData().GetContract()
	if len(contracts) == 0 {
		return nil, false
	}
	contract := contracts[0]
	ownerRaw := parser.ContractOwner(contract)
	if len(ownerRaw) == 0 {
		return nil, false
	}
	owner := parser.HexToBase58(hex.EncodeToString(ownerRaw))
	if owner == "" || owner != sub.Address {
		return nil, false
	}

	ev := &webhook.UsageEvent{
		EventType:      webhook.EventTypeEnergyUsage,
		EventID:        "usage_" + event.TransactionID + "_" + sub.SubscriptionID,
		Timestamp:      time.Now().Unix(),
		Network:        network,
		TxHash:         event.TransactionID,
		BlockNumber:    event.BlockNumber,
		BlockTimestamp: event.BlockTimestamp,
		OwnerAddress:   owner,
		ContractType:   contract.GetType().String(),
		Direction:      "outgoing",
		WalletType:     sub.WalletType,
		WatchedAddress: sub.Address,
		SubscriptionID: sub.SubscriptionID,
		UserID:         sub.UserID,
		Label:          sub.Label,
		Metadata:       sub.Metadata,
	}

	switch contract.GetType() {
	case core.Transaction_Contract_TriggerSmartContract:
		var trigger core.TriggerSmartContract
		if err := proto.Unmarshal(contract.GetParameter().GetValue(), &trigger); err == nil {
			if len(trigger.GetContractAddress()) > 0 {
				ev.ContractAddress = parser.HexToBase58(hex.EncodeToString(trigger.GetContractAddress()))
			}
			if data := trigger.GetData(); len(data) >= 4 {
				ev.MethodSignature = hex.EncodeToString(data[:4])
			}
		}
	case core.Transaction_Contract_CreateSmartContract:
		if info := event.RawTxInfo; info != nil && len(info.GetContractAddress()) > 0 {
			ev.ContractAddress = parser.HexToBase58(hex.EncodeToString(info.GetContractAddress()))
		}
	}
	if sc, ok := event.EventData["smartContract"].(map[string]interface{}); ok {
		if name, ok := sc["methodName"].(string); ok {
			ev.MethodName = name
		}
	}

	info := event.RawTxInfo
	if info == nil {
		// Outcome and receipt unknown: still a hint that the owner sent a transaction.
		ev.Result = "UNKNOWN"
		return ev, true
	}

	ev.ReceiptKnown = true
	ev.Success = monitor.TxInfoSucceeded(info)
	ev.Result = usageResult(info)
	ev.ResultMessage = printable(string(info.GetResMessage()), maxResultMessage)
	receipt := info.GetReceipt()
	ev.EnergyUsage = receipt.GetEnergyUsage()
	ev.EnergyUsageTotal = receipt.GetEnergyUsageTotal()
	ev.OriginEnergyUsage = receipt.GetOriginEnergyUsage()
	ev.EnergyPenaltyTotal = receipt.GetEnergyPenaltyTotal()
	ev.EnergyFee = receipt.GetEnergyFee()
	ev.NetUsage = receipt.GetNetUsage()
	ev.NetFee = receipt.GetNetFee()
	ev.Fee = info.GetFee()

	if ev.EnergyUsage == 0 && ev.EnergyUsageTotal == 0 && ev.OriginEnergyUsage == 0 &&
		ev.EnergyFee == 0 && ev.NetUsage == 0 && ev.NetFee == 0 && ev.Fee == 0 {
		return nil, false
	}
	return ev, true
}

// usageResult names how a transaction ended: the receipt's contract result for
// contract calls, else SUCCESS or FAILED from the transaction info.
func usageResult(info *core.TransactionInfo) string {
	if receipt := info.GetReceipt(); receipt != nil && receipt.GetResult() != core.Transaction_Result_DEFAULT {
		return receipt.GetResult().String()
	}
	if info.GetResult() == core.TransactionInfo_FAILED {
		return "FAILED"
	}
	return "SUCCESS"
}

// printable keeps the printable ASCII of s, at most limit bytes.
func printable(s string, limit int) string {
	var b strings.Builder
	for _, c := range s {
		if b.Len() >= limit {
			break
		}
		if c >= 0x20 && c < 0x7f {
			b.WriteRune(c)
		}
	}
	return strings.TrimSpace(b.String())
}

// sendsPortoTransferEvents reports whether the subscription's events go to the Porto
// transfer and operation routes. Usage-only wallet types (keep_level) never do.
func sendsPortoTransferEvents(sub *models.Subscription) bool {
	return sub != nil && sub.WalletType != WalletTypeKeepLevel
}
