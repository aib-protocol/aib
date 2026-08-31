// Package utxo — on-chain release anchoring (RFC-distribution-D).
//
// A release record is carried by an ordinary (signed, fee-paying)
// transaction whose FIRST output is a zero-value output to a
// deterministic "anchor address". The payload (version tag + SHA256 of
// the release artifacts) lives in that output's Script field. Old nodes
// validate it as a normal 0-value output (no rule rejects it), so this
// is fully backward compatible — no chain split.
//
// Script layout (version-gated, max 128 bytes):
//   [0]    = 0xA1  magic ("AIB anchor")
//   [1]    = payload version (1)
//   [2]    = name length N (tag e.g. "v0.11.23-testnet", N <= 32)
//   [3:3+N]= name ASCII
//   rest   = 32-byte SHA256 of the release binary
package utxo

import (
	"crypto/sha256"
	"errors"
	"fmt"
)

// AnchorMagic marks a release-anchor output script.
const AnchorMagic byte = 0xA1

// AnchorScriptVersion is the current anchor payload version.
const AnchorScriptVersion byte = 1

// AnchorAddress is the deterministic destination for anchor outputs.
// Anyone may verify the anchor was "paid" to this well-known address.
// = AddressFromPublicKey(SHA256("aib-release-anchor-v1"))
var AnchorAddress = computeAnchorAddress()

func computeAnchorAddress() [32]byte {
	h := sha256.Sum256([]byte("aib-release-anchor-v1"))
	var a [32]byte
	copy(a[:], h[:])
	return a
}

// ErrNotAnchor indicates a script that is not a release anchor.
var ErrNotAnchor = errors.New("not a release anchor")

// BuildAnchorScript encodes a release name and artifact SHA256 into an
// anchor output script.
func BuildAnchorScript(name string, sha [32]byte) []byte {
	n := len(name)
	if n > 32 {
		n = 32
	}
	s := make([]byte, 0, 3+n+32)
	s = append(s, AnchorMagic, AnchorScriptVersion, byte(n))
	s = append(s, []byte(name[:n])...)
	s = append(s, sha[:]...)
	return s
}

// ParseAnchorScript decodes an anchor script. Returns name + SHA256.
func ParseAnchorScript(script []byte) (string, [32]byte, error) {
	var sha [32]byte
	if len(script) < 3+32 {
		return "", sha, ErrNotAnchor
	}
	if script[0] != AnchorMagic {
		return "", sha, ErrNotAnchor
	}
	if script[1] != AnchorScriptVersion {
		return "", sha, fmt.Errorf("anchor version %d unsupported", script[1])
	}
	n := int(script[2])
	if n > 32 || len(script) < 3+n+32 {
		return "", sha, ErrNotAnchor
	}
	name := string(script[3 : 3+n])
	copy(sha[:], script[3+n:3+n+32])
	return name, sha, nil
}

// IsAnchorOutput reports whether a transaction output is an anchor.
func IsAnchorOutput(o TXOutput) bool {
	_, _, err := ParseAnchorScript(o.Script)
	return err == nil
}

// ReleaseRecord is a decoded on-chain release anchor.
type ReleaseRecord struct {
	Name   string `json:"name"`    // e.g. "v0.11.23-testnet"
	SHA256 string `json:"sha256"`  // hex, 64 chars
	Height uint64 `json:"height"`  // block containing the anchor
	TxHash string `json:"tx_hash"` // hex
}
