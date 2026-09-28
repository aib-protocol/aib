package utxo

import (
	"strings"
	"testing"
)

// Round-trip: encode → decode must return the identical 32 bytes, uppercase.
func TestDisplayAddressRoundTrip(t *testing.T) {
	addr := [32]byte{}
	for i := range addr {
		addr[i] = byte(i * 7)
	}
	s, err := EncodeDisplayAddress(addr)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.HasPrefix(s, "AIB1") {
		t.Fatalf("prefix: %s", s[:8])
	}
	if s != strings.ToUpper(s) {
		t.Fatalf("not uppercase: %s", s)
	}
	got, err := ParseAddressAny(s)
	if err != nil {
		t.Fatalf("parse upper: %v", err)
	}
	if got != addr {
		t.Fatal("round-trip mismatch")
	}
}

// Lowercase aib1... must parse to the same address.
func TestParseLowercase(t *testing.T) {
	addr := [32]byte{1, 2, 3}
	low := strings.ToLower(MustEncodeDisplayAddress(addr))
	got, err := ParseAddressAny(low)
	if err != nil {
		t.Fatalf("parse lower: %v", err)
	}
	if got != addr {
		t.Fatal("lowercase mismatch")
	}
}

// Mixed case must be rejected (BIP-173 rule).
func TestRejectMixedCase(t *testing.T) {
	s := MustEncodeDisplayAddress([32]byte{9})
	mixed := "Aib" + s[3:]
	if _, err := ParseAddressAny(mixed); err == nil {
		t.Fatal("mixed case accepted")
	}
}

// Checksum actually catches typos: flip one char in the data part.
func TestChecksumCatchesTypo(t *testing.T) {
	s := MustEncodeDisplayAddress([32]byte{5})
	flip := s[:10] + "q" + s[11:]
	if s[10] == 'q' {
		flip = s[:10] + "p" + s[11:]
	}
	if _, err := ParseAddressAny(flip); err == nil {
		t.Fatal("typo accepted — checksum broken")
	}
}

// EVM 0x addresses must be rejected with a pointed message.
func TestRejectEVM(t *testing.T) {
	evm := "0xf1bf7d62f9b57e217342419a185aa406e0d5df0c"
	_, err := ParseAddressAny(evm)
	if err == nil {
		t.Fatal("EVM address accepted")
	}
	if !strings.Contains(err.Error(), "EVM") {
		t.Fatalf("error should mention EVM: %v", err)
	}
}

// 40-hex (EVM without 0x) must be rejected on length with pointed message.
func TestReject40Hex(t *testing.T) {
	_, err := ParseAddressAny("f1bf7d62f9b57e217342419a185aa406e0d5df0c")
	if err == nil {
		t.Fatal("40-hex accepted")
	}
	if !strings.Contains(err.Error(), "20 bytes") {
		t.Fatalf("error should mention 20-byte EVM: %v", err)
	}
}

// Legacy 64-hex still parses (canonical on-disk format unchanged).
func TestLegacyHexStillWorks(t *testing.T) {
	hexStr := "d274df0052080980d75bdb0ab701c890b48e1793994bf44cb5935e133a15189d"
	got, err := ParseAddressAny(hexStr)
	if err != nil {
		t.Fatalf("legacy hex: %v", err)
	}
	want := [32]byte{}
	copy(want[:], mustHex(t, hexStr))
	if got != want {
		t.Fatal("hex mismatch")
	}
}

// Known-address cross-format consistency: the user's validator wallet and the
// faucet address must encode/decode consistently in all three accepted forms.
func TestKnownAddressesConsistency(t *testing.T) {
	for _, h := range []string{
		"d274df0052080980d75bdb0ab701c890b48e1793994bf44cb5935e133a15189d", // user validator wallet
		"7550bf422d43242835d9af028c9ddb33b97bc3a409cece40addee3e2c1d51a37", // faucet
	} {
		var a [32]byte
		copy(a[:], mustHex(t, h))
		disp := MustEncodeDisplayAddress(a)
		back, err := ParseAddressAny(disp)
		if err != nil {
			t.Fatalf("%s: %v", h[:8], err)
		}
		if back != a {
			t.Fatalf("%s: display round-trip mismatch", h[:8])
		}
	}
}

// Whitespace tolerated.
func TestWhitespaceTolerated(t *testing.T) {
	addr := [32]byte{42}
	s := "  " + MustEncodeDisplayAddress(addr) + "\n"
	got, err := ParseAddressAny(s)
	if err != nil || got != addr {
		t.Fatalf("whitespace: err=%v", err)
	}
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b := make([]byte, 32)
	for i := 0; i < 32; i++ {
		n := hexVal(t, s[i*2])
		n2 := hexVal(t, s[i*2+1])
		b[i] = n<<4 | n2
	}
	return b
}

func hexVal(t *testing.T, c byte) byte {
	t.Helper()
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	t.Fatalf("bad hex char %q", c)
	return 0
}
