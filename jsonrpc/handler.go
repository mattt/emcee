package jsonrpc

// Handler defines the interface for handling JSON-RPC requests
type Handler interface {
	Handle(request Request) Response
}
