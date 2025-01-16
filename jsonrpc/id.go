package jsonrpc

import (
	"encoding/json"
	"fmt"
)

// ID represents a JSON-RPC ID which must be either a string or number
type ID struct {
	value interface{}
}

// NewID creates a JSON-RPC ID from a string or number
func NewID(id interface{}) (ID, error) {
	switch v := id.(type) {
	case ID:
		return v, nil
	case string:
		return ID{value: v}, nil
	case int, int32, int64, float32, float64:
		return ID{value: v}, nil
	case nil:
		return ID{}, fmt.Errorf("id cannot be null")
	default:
		return ID{}, fmt.Errorf("id must be string or number, got %T", id)
	}
}

func (id ID) Value() interface{} {
	return id.value
}

func (id ID) IsNil() bool {
	return id.value == nil
}

// Equal compares two IDs for equality
func (id ID) Equal(other interface{}) bool {
	// If comparing with raw value
	switch v := other.(type) {
	case string, int, int32, int64, float32, float64:
		return id.value == v
	case ID:
		return id.value == v.value
	default:
		return false
	}
}

var _ fmt.GoStringer = ID{}

// GoString implements fmt.GoStringer
func (id ID) GoString() string {
	switch v := id.value.(type) {
	case string:
		return fmt.Sprintf("%q", v)
	case float64, float32:
		return fmt.Sprintf("%g", v)
	case int, int32, int64:
		return fmt.Sprintf("%d", v)
	case nil:
		return "nil"
	default:
		return fmt.Sprintf("%v", v)
	}
}

var _ json.Marshaler = ID{}

func (id ID) MarshalJSON() ([]byte, error) {
	switch id.value {
	case nil:
		return json.Marshal(0)
	default:
		return json.Marshal(id.value)
	}
}

var _ json.Unmarshaler = &ID{}

// UnmarshalJSON implements json.Unmarshaler
func (id *ID) UnmarshalJSON(data []byte) error {
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	switch v := raw.(type) {
	case string:
		id.value = v
		return nil
	case float64: // JSON numbers are decoded as float64
		id.value = int(v)
		return nil
	case nil:
		return fmt.Errorf("id cannot be null")
	default:
		return fmt.Errorf("id must be string or number, got %T", raw)
	}
}
