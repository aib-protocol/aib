package abi

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// Argument represents a function argument
type Argument struct {
	Name    string
	Type    string
	Indexed bool // for events
}

// Method represents a contract method
type Method struct {
	Name    string
	Inputs  []Argument
	Outputs []Argument
}

// Event represents a contract event
type Event struct {
	Name   string
	Inputs []Argument
}

// ABI represents a contract ABI
type ABI struct {
	Methods map[string]Method
	Events  map[string]Event
}

// NewABI creates a new ABI from JSON
func NewABI(jsonData []byte) (ABI, error) {
	var abi ABI
	abi.Methods = make(map[string]Method)
	abi.Events = make(map[string]Event)

	err := json.Unmarshal(jsonData, &abi)
	if err != nil {
		return ABI{}, fmt.Errorf("failed to unmarshal ABI: %w", err)
	}

	return abi, nil
}

// Pack packs the given method name and arguments
func (abi *ABI) Pack(name string, args ...interface{}) ([]byte, error) {
	method, exist := abi.Methods[name]
	if !exist {
		return nil, fmt.Errorf("method '%s' not found", name)
	}

	arguments := make([]byte, 0)

	// Method ID (first 4 bytes of keccak256(methodSignature))
	methodID := method.ID()
	arguments = append(arguments, methodID...)

	// Pack arguments (simplified - just append for now)
	for i, arg := range method.Inputs {
		if i < len(args) {
			packed, _ := packValue(arg.Type, args[i])
			arguments = append(arguments, packed...)
		}
	}

	return arguments, nil
}

// Unpack unpacks the given output data
func (abi *ABI) Unpack(name string, data []byte) ([]interface{}, error) {
	method, exist := abi.Methods[name]
	if !exist {
		return nil, fmt.Errorf("method '%s' not found", name)
	}

	results := make([]interface{}, len(method.Outputs))

	for i, output := range method.Outputs {
		unpacked, _ := unpackValue(output.Type, data)
		results[i] = unpacked
	}

	return results, nil
}

// ID returns the method ID
func (m *Method) ID() []byte {
	signature := m.String()
	hash := common.FromHex(signature)
	return hash[:4]
}

// String returns the method signature
func (m *Method) String() string {
	inputs := make([]string, len(m.Inputs))
	for i, input := range m.Inputs {
		inputs[i] = input.Type
	}

	outputs := make([]string, len(m.Outputs))
	for i, output := range m.Outputs {
		outputs[i] = output.Type
	}

	return fmt.Sprintf("%s(%s)%s", m.Name, strings.Join(inputs, ","), strings.Join(outputs, ","))
}

// packValue packs a value based on its type
func packValue(typeStr string, value interface{}) ([]byte, error) {
	switch typeStr {
	case "address":
		if addr, ok := value.(common.Address); ok {
			return addr.Bytes(), nil
		}
		return make([]byte, 20), nil
	case "uint256", "uint":
		if n, ok := value.(string); ok {
			// Parse as hex
			if bytes := common.FromHex(n); len(bytes) > 0 {
				return bytes, nil
			}
		}
		return make([]byte, 32), nil
	case "bool":
		if b, ok := value.(bool); ok {
			result := make([]byte, 32)
			if b {
				result[31] = 1
			}
			return result, nil
		}
		return make([]byte, 32), nil
	case "string":
		if s, ok := value.(string); ok {
			return []byte(s), nil
		}
		return nil, nil
	default:
		return nil, nil
	}
}

// unpackValue unpacks a value based on its type
func unpackValue(typeStr string, data []byte) (interface{}, error) {
	switch typeStr {
	case "address":
		if len(data) >= 20 {
			return common.BytesToAddress(data[:20]), nil
		}
		return common.Address{}, nil
	case "uint256", "uint":
		return data, nil
	case "bool":
		if len(data) > 0 {
			return data[len(data)-1] == 1, nil
		}
		return false, nil
	case "string":
		return string(data), nil
	default:
		return data, nil
	}
}

// Encode encodes the given data according to the ABI
func Encode(typeString string, value interface{}) ([]byte, error) {
	return packValue(typeString, value)
}

// Decode decodes the given data according to the ABI
func Decode(typeString string, data []byte) (interface{}, error) {
	return unpackValue(typeString, data)
}
