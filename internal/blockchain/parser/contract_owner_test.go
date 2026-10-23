package parser

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func contractOf(t *testing.T, typ core.Transaction_Contract_ContractType, msg proto.Message) *core.Transaction_Contract {
	t.Helper()
	param, err := anypb.New(msg)
	if err != nil {
		t.Fatal(err)
	}
	return &core.Transaction_Contract{Type: typ, Parameter: param}
}

func TestContractOwnerOfEveryContractType(t *testing.T) {
	owner := append([]byte{0x41}, bytes.Repeat([]byte{0x11}, 20)...)
	other := append([]byte{0x41}, bytes.Repeat([]byte{0x22}, 20)...)

	cases := []struct {
		name     string
		contract *core.Transaction_Contract
	}{
		{"trx transfer", contractOf(t, core.Transaction_Contract_TransferContract, &core.TransferContract{OwnerAddress: owner, ToAddress: other, Amount: 1})},
		// owner_address is field 2 here: the typed message, not a field number, finds it
		{"trc10 transfer", contractOf(t, core.Transaction_Contract_TransferAssetContract, &core.TransferAssetContract{AssetName: []byte("1002000"), OwnerAddress: owner, ToAddress: other})},
		{"contract call", contractOf(t, core.Transaction_Contract_TriggerSmartContract, &core.TriggerSmartContract{OwnerAddress: owner, ContractAddress: other})},
		{"account update", contractOf(t, core.Transaction_Contract_AccountUpdateContract, &core.AccountUpdateContract{OwnerAddress: owner, AccountName: []byte("x")})},
		{"account create", contractOf(t, core.Transaction_Contract_AccountCreateContract, &core.AccountCreateContract{OwnerAddress: owner, AccountAddress: other})},
		{"delegate", contractOf(t, core.Transaction_Contract_DelegateResourceContract, &core.DelegateResourceContract{OwnerAddress: owner, ReceiverAddress: other})},
	}
	for _, c := range cases {
		if got := ContractOwner(c.contract); !bytes.Equal(got, owner) {
			t.Errorf("%s: ContractOwner = %x, want %x", c.name, got, owner)
		}
	}

	if ContractOwner(&core.Transaction_Contract{}) != nil {
		t.Error("a contract without parameter has no owner")
	}
}

func TestExtractAddressesFindsTheOwnerOfGenericContracts(t *testing.T) {
	owner := append([]byte{0x41}, bytes.Repeat([]byte{0x33}, 20)...)
	p := NewTronParser(nil)
	got := p.ExtractAddresses(contractOf(t, core.Transaction_Contract_AccountUpdateContract, &core.AccountUpdateContract{OwnerAddress: owner}))
	if len(got) != 1 || got[0] != hex.EncodeToString(owner) {
		t.Fatalf("ExtractAddresses = %v, want the owner %x", got, owner)
	}
}
