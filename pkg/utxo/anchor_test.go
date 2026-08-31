package utxo

import "testing"

func TestAnchorRoundTrip(t *testing.T) {
	name := "v0.11.23-testnet"
	var sha [32]byte
	for i := range sha {
		sha[i] = byte(i)
	}
	s := BuildAnchorScript(name, sha, [32]byte{})
	gotName, gotSha, _, err := ParseAnchorScript(s)
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
	if _, _, _, err := ParseAnchorScript([]byte{0x00, 0x01}); err == nil {
		t.Fatal("short script accepted")
	}
	if _, _, _, err := ParseAnchorScript([]byte{0x6a, 0x01, 0}); err == nil {
		t.Fatal("wrong magic accepted")
	}
}

func TestAnchorV2RoundTrip(t *testing.T) {
	name := "v0.11.26-testnet"
	var binSHA, insSHA [32]byte
	for i := range binSHA {
		binSHA[i] = byte(i + 1)
		insSHA[i] = byte(255 - i)
	}
	script := BuildAnchorScript(name, binSHA, insSHA)
	gotName, gotBin, gotIns, err := ParseAnchorScript(script)
	if err != nil {
		t.Fatalf("parse v2: %v", err)
	}
	if gotName != name || gotBin != binSHA || gotIns != insSHA {
		t.Fatal("v2 round-trip mismatch")
	}
	// v1 script must still parse
	v1 := append([]byte{AnchorMagic, 1, byte(len(name))}, []byte(name)...)
	v1 = append(v1, binSHA[:]...)
	gotName1, gotBin1, gotIns1, err1 := ParseAnchorScript(v1)
	if err1 != nil || gotName1 != name || gotBin1 != binSHA || gotIns1 != ([32]byte{}) {
		t.Fatal("v1 backward-compat parse failed")
	}
}
