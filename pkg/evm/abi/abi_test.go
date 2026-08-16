package abi

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackAddress(t *testing.T) {
	addr := common.HexToAddress("0x1234567890123456789012345678901234567890")

	packed, err := packValue("address", addr)
	require.NoError(t, err)
	require.Len(t, packed, 20)
}

func TestPackBool(t *testing.T) {
	packed, err := packValue("bool", true)
	require.NoError(t, err)
	require.NotEmpty(t, packed)

	packed2, err := packValue("bool", false)
	require.NoError(t, err)
	require.NotNil(t, packed2)
}

func TestPackString(t *testing.T) {
	packed, err := packValue("string", "hello")
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), packed)
}

func TestUnpackAddress(t *testing.T) {
	data := common.Hex2Bytes("1234567890123456789012345678901234567890" + "000000000000000000000000")

	unpacked, err := unpackValue("address", data)
	require.NoError(t, err)
	addr, ok := unpacked.(common.Address)
	require.True(t, ok)
	assert.Equal(t, "0x1234567890123456789012345678901234567890", addr.Hex())
}

func TestEncode(t *testing.T) {
	encoded, err := Encode("string", "test")
	require.NoError(t, err)
	assert.Equal(t, []byte("test"), encoded)
}

func TestDecode(t *testing.T) {
	decoded, err := Decode("string", []byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, "hello", decoded)
}
