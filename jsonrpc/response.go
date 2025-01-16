package jsonrpc

// Response represents a JSON-RPC response object
type Response struct {
	Version string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
	ID      ID          `json:"id"`
}

// NewResponse creates a new Response object
func NewResponse(id interface{}, result interface{}, err *Error) Response {
	respID, _ := NewID(id)

	return Response{
		Version: Version,
		ID:      respID,
		Result:  result,
		Error:   err,
	}
}
