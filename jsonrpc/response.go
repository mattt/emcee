package jsonrpc

// Result represents a map of string keys to arbitrary values
type Result interface{}

// Response represents a JSON-RPC response object
type Response struct {
	Version string `json:"jsonrpc"`
	Result  Result `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
	ID      ID     `json:"id"`
}

// NewResponse creates a new Response object
func NewResponse(id interface{}, result Result, err *Error) Response {
	respID, _ := NewID(id)

	return Response{
		Version: Version,
		ID:      respID,
		Result:  result,
		Error:   err,
	}
}
