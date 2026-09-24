package client

import (
	"strings"
	"testing"
)

func TestTxIDBytesDecodesHex(t *testing.T) {
	const txID = "e8db4a17365a17c750e4235cd9a8f43842d9ea814a0a710a948f22a1b531cf06"
	for _, in := range []string{txID, "0x" + txID, strings.ToUpper(txID), " " + txID + " "} {
		id, err := TxIDBytes(in)
		if err != nil {
			t.Fatalf("TxIDBytes(%q): %v", in, err)
		}
		if len(id) != 32 || id[0] != 0xe8 || id[31] != 0x06 {
			t.Fatalf("TxIDBytes(%q) = %x, want the 32 decoded bytes", in, id)
		}
	}
}

func TestTxIDBytesRejectsBadIDs(t *testing.T) {
	for _, in := range []string{"", "xyz", "abcd", strings.Repeat("ab", 33)} {
		if _, err := TxIDBytes(in); err == nil {
			t.Errorf("TxIDBytes(%q): want an error", in)
		}
	}
}
