package monitor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fbsobreira/gotron-sdk/pkg/common"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/frstrtr/mongotron/internal/blockchain/client"
	"github.com/frstrtr/mongotron/internal/blockchain/parser"
	"github.com/frstrtr/mongotron/pkg/logger"
	"google.golang.org/protobuf/types/known/anypb"
)

// fakeChain is an in-memory node: blocks by number, transaction infos by id, and a
// count of every read, so tests can assert how much the monitors ask the node for.
type fakeChain struct {
	mu     sync.Mutex
	parser *parser.TronParser
	head   int64
	blocks map[int64]*core.Block
	infos  map[string]*core.TransactionInfo

	headerUnsupported bool
	failByNum         map[int64]int // remaining failures per block number
	infoGate          chan struct{} // when set, transaction info reads wait for it

	nowCalls    int
	byNumCalls  int
	headerCalls int
	infoCalls   int
	byNumBlocks map[int64]int
}

func testLog() *logger.Logger {
	l := logger.NewDefault()
	return &l
}

func newFakeChain(head int64) *fakeChain {
	c := &fakeChain{
		parser:      parser.NewTronParser(testLog()),
		blocks:      make(map[int64]*core.Block),
		infos:       make(map[string]*core.TransactionInfo),
		failByNum:   make(map[int64]int),
		byNumBlocks: make(map[int64]int),
	}
	// Blocks up to the head exist (empty)
	for n := head - 50; n <= head; n++ {
		c.blocks[n] = makeBlock(n)
	}
	c.head = head
	return c
}

func makeBlock(num int64, txs ...*core.Transaction) *core.Block {
	return &core.Block{
		BlockHeader: &core.BlockHeader{
			RawData:          &core.BlockHeaderRaw{Number: num, Timestamp: 1_760_000_000_000 + num*3000},
			WitnessSignature: []byte(fmt.Sprintf("sig-%d", num)),
		},
		Transactions: txs,
	}
}

// addBlock appends a block with txs (and a successful info for each tx).
func (c *fakeChain) addBlock(txs ...*core.Transaction) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.head++
	c.blocks[c.head] = makeBlock(c.head, txs...)
	for _, tx := range txs {
		id := c.parser.CalculateTransactionID(tx)
		if _, ok := c.infos[id]; !ok {
			c.infos[id] = successInfo(id, c.head)
		}
	}
	return c.head
}

func successInfo(id string, block int64) *core.TransactionInfo {
	raw, _ := client.TxIDBytes(id)
	return &core.TransactionInfo{
		Id:          raw,
		BlockNumber: block,
		Result:      core.TransactionInfo_SUCESS,
		Receipt:     &core.ResourceReceipt{NetUsage: 268, Result: core.Transaction_Result_SUCCESS},
	}
}

func (c *fakeChain) GetNowBlock(ctx context.Context) (*core.Block, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nowCalls++
	return c.blocks[c.head], nil
}

func (c *fakeChain) GetBlockByNum(ctx context.Context, n int64) (*core.Block, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byNumCalls++
	c.byNumBlocks[n]++
	if left := c.failByNum[n]; left > 0 {
		c.failByNum[n] = left - 1
		return nil, errors.New("transient node error")
	}
	b, ok := c.blocks[n]
	if !ok || n > c.head {
		return nil, fmt.Errorf("block %d not found", n)
	}
	return b, nil
}

func (c *fakeChain) GetTransactionInfoById(ctx context.Context, id string) (*core.TransactionInfo, error) {
	c.mu.Lock()
	gate := c.infoGate
	c.infoCalls++
	info, ok := c.infos[id]
	c.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if !ok {
		return nil, client.ErrTransactionInfoNotFound
	}
	return info, nil
}

func (c *fakeChain) GetHeadBlockNumber(ctx context.Context) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.headerCalls++
	if c.headerUnsupported {
		return 0, fmt.Errorf("%w: rpc error: code = Unimplemented", client.ErrHeaderOnlyUnsupported)
	}
	return c.head, nil
}

type chainCounts struct{ now, byNum, header, info int }

func (c *fakeChain) counts() chainCounts {
	c.mu.Lock()
	defer c.mu.Unlock()
	return chainCounts{c.nowCalls, c.byNumCalls, c.headerCalls, c.infoCalls}
}

// fullHeadOnly exposes only the BlockSource methods (no header-only head read), like a
// client the monitors used before the scanner existed.
type fullHeadOnly struct{ c *fakeChain }

func (f fullHeadOnly) GetNowBlock(ctx context.Context) (*core.Block, error) {
	return f.c.GetNowBlock(ctx)
}
func (f fullHeadOnly) GetBlockByNum(ctx context.Context, n int64) (*core.Block, error) {
	return f.c.GetBlockByNum(ctx, n)
}
func (f fullHeadOnly) GetTransactionInfoById(ctx context.Context, id string) (*core.TransactionInfo, error) {
	return f.c.GetTransactionInfoById(ctx, id)
}

// testAddr returns a deterministic TRON address: raw 21 bytes and base58.
func testAddr(i int) ([]byte, string) {
	raw := make([]byte, 21)
	raw[0] = 0x41
	raw[1] = 0x7a
	raw[19] = byte(i >> 8)
	raw[20] = byte(i)
	return raw, common.EncodeCheck(raw)
}

var txNonce int64

func trxTransferTx(from, to []byte, amount int64) *core.Transaction {
	txNonce++
	param, _ := anypb.New(&core.TransferContract{OwnerAddress: from, ToAddress: to, Amount: amount})
	return &core.Transaction{RawData: &core.TransactionRaw{
		Timestamp: txNonce,
		Contract:  []*core.Transaction_Contract{{Type: core.Transaction_Contract_TransferContract, Parameter: param}},
	}}
}

func triggerTx(owner, contract, data []byte) *core.Transaction {
	txNonce++
	param, _ := anypb.New(&core.TriggerSmartContract{OwnerAddress: owner, ContractAddress: contract, Data: data})
	return &core.Transaction{RawData: &core.TransactionRaw{
		Timestamp: txNonce,
		Contract:  []*core.Transaction_Contract{{Type: core.Transaction_Contract_TriggerSmartContract, Parameter: param}},
	}}
}

func txID(tx *core.Transaction) string {
	return parser.NewTronParser(testLog()).CalculateTransactionID(tx)
}

// collect reads n events from ch or fails after the timeout.
func collect(t *testing.T, ch <-chan *AddressEvent, n int) []*AddressEvent {
	t.Helper()
	out := make([]*AddressEvent, 0, n)
	deadline := time.After(5 * time.Second)
	for len(out) < n {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("event channel closed after %d of %d events", len(out), n)
			}
			out = append(out, ev)
		case <-deadline:
			t.Fatalf("timed out after %d of %d events", len(out), n)
		}
	}
	return out
}

// expectNone fails when an event arrives within d.
func expectNone(t *testing.T, ch <-chan *AddressEvent, d time.Duration) {
	t.Helper()
	select {
	case ev := <-ch:
		t.Fatalf("unexpected event: block %d tx %s", ev.BlockNumber, ev.TransactionID)
	case <-time.After(d):
	}
}

// waitUntil polls cond until it holds or fails after 5 s.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}
