package jsonrpc

// Response represents a JSON-RPC response object
type Response struct {
	Version string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
	Id      interface{} `json:"id"`
}

// NewResponse creates a new Response object
func NewResponse(id interface{}, result interface{}, err *Error) Response {
	return Response{
		Version: Version,
		Result:  result,
		Error:   err,
		Id:      id,
	}
}
