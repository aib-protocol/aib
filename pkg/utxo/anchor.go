// Package utxo — on-chain release anchoring (RFC-distribution-D).
//
// A release record is carried by an ordinary (signed, fee-paying)
// transaction whose FIRST output is a zero-value output to a
// deterministic "anchor address". The payload (version tag + SHA256 of
// the release artifacts) lives in that output's Script field. Old nodes
// validate it as a normal 0-value output (no rule rejects it), so this
// is fully backward compatible — no chain split.
//
// Script v1 layout (legacy, max 128 bytes):
//
//	[0]=0xA1 magic, [1]=1 version, [2]=name len N,
//	[3:3+N]=name, rest = 32-byte SHA256 of the release binary
//
// Script v2 layout (adds installer hash):
//
//	[0]=0xA1 magic, [1]=2 version, [2]=name len N,
//	[3:3+N]=name, next 32 = binary SHA256, next 32 = install.sh SHA256
package utxo

import (
	"crypto/sha256"
	"errors"
	"fmt"
)

// AnchorMagic marks a release-anchor output script.
const AnchorMagic byte = 0xA1

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

// BuildAnchorScript encodes release name + artifact hashes (v2).
// installerSHA may be all-zero for binary-only anchors.
func BuildAnchorScript(name string, binarySHA, installerSHA [32]byte) []byte {
	n := len(name)
	if n > 32 {
		n = 32
	}
	s := make([]byte, 0, 3+n+64)
	s = append(s, AnchorMagic, 2, byte(n))
	s = append(s, []byte(name[:n])...)
	s = append(s, binarySHA[:]...)
	s = append(s, installerSHA[:]...)
	return s
}

// ParseAnchorScript decodes an anchor script (v1 or v2).
// v1 yields a zero installer hash.
func ParseAnchorScript(script []byte) (name string, binarySHA, installerSHA [32]byte, err error) {
	if len(script) < 3 {
		return "", binarySHA, installerSHA, ErrNotAnchor
	}
	if script[0] != AnchorMagic {
		return "", binarySHA, installerSHA, ErrNotAnchor
	}
	ver := script[1]
	if ver != 1 && ver != 2 {
		return "", binarySHA, installerSHA, fmt.Errorf("anchor version %d unsupported", ver)
	}
	n := int(script[2])
	if n > 32 || len(script) < 3+n+32 {
		return "", binarySHA, installerSHA, ErrNotAnchor
	}
	name = string(script[3 : 3+n])
	copy(binarySHA[:], script[3+n:3+n+32])
	if ver == 2 && len(script) >= 3+n+64 {
		copy(installerSHA[:], script[3+n+32:3+n+64])
	}
	return name, binarySHA, installerSHA, nil
}

// IsAnchorOutput reports whether a transaction output is an anchor.
func IsAnchorOutput(o TXOutput) bool {
	_, _, _, err := ParseAnchorScript(o.Script)
	return err == nil
}

// ReleaseRecord is a decoded on-chain release anchor.
type ReleaseRecord struct {
	Name            string `json:"name"`             // e.g. "v0.11.25-testnet"
	SHA256          string `json:"sha256"`           // binary SHA256, hex
	InstallerSHA256 string `json:"installer_sha256"` // install.sh SHA256, hex ("" for v1 anchors)
	Height          uint64 `json:"height"`           // block containing the anchor
	TxHash          string `json:"tx_hash"`          // hex
}
