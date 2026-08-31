package utxo

import "testing"

func TestAnchorIndexScan(t *testing.T) {
	ai := NewAnchorIndex()
	var sha [32]byte
	sha[0] = 0xAA
	b := &Block{
		Header: BlockHeader{Height: 10880},
		Transactions: []*Transaction{{
			Version: 1,
			Outputs: []TXOutput{{Value: 0, Script: BuildAnchorScript("v0.11.23-testnet", sha)}},
		}},
	}
	ai.ScanBlock(b)
	if ai.Latest() == nil || ai.Latest().Name != "v0.11.23-testnet" {
		t.Fatal("anchor not indexed")
	}
	if len(ai.History()) != 1 {
		t.Fatal("history wrong")
	}
	// non-anchor block ignored
	ai.ScanBlock(&Block{Header: BlockHeader{Height: 10881}, Transactions: []*Transaction{{Version: 1}}})
	if len(ai.History()) != 1 {
		t.Fatal("false positive")
	}
}
