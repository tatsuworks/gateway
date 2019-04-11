package gateway

// Receiver receives events from a Discord websocket.
// Receivers must make sure to not handle events asynchronously since
// events must be handled in order per connection. Receivers must
// copy Events if it will be used after returning.
type Receiver interface {
	Receive(*Event) error
}

type Event struct {
	Data []byte
	Op   int
	Seq  int
	Type string
}
