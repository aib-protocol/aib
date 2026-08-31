package utxo

import "testing"

func TestAnchorRoundTrip(t *testing.T) {
	name := "v0.11.23-testnet"
	var sha [32]byte
	for i := range sha {
		sha[i] = byte(i)
	}
	s := BuildAnchorScript(name, sha)
	gotName, gotSha, err := ParseAnchorScript(s)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if gotName != name {
		t.Fatalf("name %q != %q", gotName, name)
	}
	if gotSha != sha {
		t.Fatal("sha mismatch")
	}
}

func TestAnchorRejectsGarbage(t *testing.T) {
	if _, _, err := ParseAnchorScript([]byte{0x00, 0x01}); err == nil {
		t.Fatal("short script accepted")
	}
	if _, _, err := ParseAnchorScript([]byte{0x6a, 0x01, 0}); err == nil {
		t.Fatal("wrong magic accepted")
	}
}
