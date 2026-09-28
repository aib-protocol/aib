package utxo

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// Display-layer address format: Bech32m (BIP 350) with UPPERCASE HRP "AIB".
//
// The on-chain identity is the raw 32-byte ed25519-derived address (hex in
// APIs). The AIB1... format is a display/entry layer ONLY — identical bytes,
// plus a 6-char checksum that rejects typos, plus a recognizable prefix that
// cannot be confused with EVM 0x addresses.
//
// Per BIP-173/350, a bech32 string must be entirely lowercase or entirely
// uppercase. We emit uppercase (AIB1QQ...) and accept BOTH cases on parse,
// rejecting mixed case.

// EncodeDisplayAddress renders a 32-byte address as uppercase AIB1... bech32m.
func EncodeDisplayAddress(addr [32]byte) (string, error) {
	s, err := EncodeBech32m(Bech32mHRP, addr[:])
	if err != nil {
		return "", err
	}
	return strings.ToUpper(s), nil
}

// MustEncodeDisplayAddress is EncodeDisplayAddress that panics on impossible
// errors (32-byte input always encodes). For tests and display code.
func MustEncodeDisplayAddress(addr [32]byte) string {
	s, err := EncodeDisplayAddress(addr)
	if err != nil {
		panic(err)
	}
	return s
}

// ParseAddressAny accepts every supported address spelling and returns the
// canonical 32-byte address:
//   - AIB1... / aib1...  (bech32m display format, checksum verified)
//   - 64-char hex string (legacy raw format, still the on-disk/API canonical)
//
// Rejected: mixed case bech32, wrong HRP, bad checksum, wrong length, 0x-prefixed
// EVM-style strings, and anything else that is not exactly one of the above.
func ParseAddressAny(s string) ([32]byte, error) {
	var out [32]byte
	s = strings.TrimSpace(s)

	// Explicit 0x rejection with a dedicated message: EVM addresses are the
	// most common cross-chain confusion and deserve a pointed error.
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return out, fmt.Errorf("EVM-style 0x address not valid on AIB: AIB uses ed25519 32-byte addresses (AIB1... bech32m or 64-char hex), not 20-byte EVM addresses")
	}

	// bech32m display form
	low := strings.ToLower(s)
	if strings.HasPrefix(low, Bech32mHRP+"1") {
		if low != s && strings.ToUpper(s) != s {
			return out, fmt.Errorf("mixed-case bech32 address: use either all-uppercase AIB1... or all-lowercase aib1...")
		}
		hrp, data, err := DecodeBech32m(low)
		if err != nil {
			return out, fmt.Errorf("invalid AIB1 address (checksum or format): %w", err)
		}
		if hrp != Bech32mHRP {
			return out, fmt.Errorf("unexpected address prefix %q: AIB addresses start with AIB1/aib1", hrp)
		}
		copy(out[:], data)
		return out, nil
	}

	// legacy raw hex
	b, err := hex.DecodeString(s)
	if err != nil {
		return out, fmt.Errorf("unrecognized address format: expected AIB1... bech32m or 64-char hex, got %q", clip(s))
	}
	if len(b) != 32 {
		return out, fmt.Errorf("address must be 32 bytes (64 hex chars), got %d bytes — note EVM 0x addresses are 20 bytes and NOT valid on AIB", len(b))
	}
	copy(out[:], b)
	return out, nil
}

// IsDisplayAddress reports whether s looks like an AIB1/aib1 bech32m string
// (prefix check only; use ParseAddressAny for full validation).
func IsDisplayAddress(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(low, Bech32mHRP+"1")
}

func clip(s string) string {
	if len(s) > 20 {
		return s[:20] + "..."
	}
	return s
}
