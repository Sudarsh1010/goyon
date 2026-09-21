package par

import "fmt"

// PanicError wraps a panic recovered from a work function. The panic never
// crashes the process; it is returned by the top-level call, after the
// remaining work functions have been cancelled (P6).
type PanicError struct {
	// Value is the recovered panic value.
	Value any

	// Stack is the goroutine stack captured at the panic site.
	Stack []byte

	// Index is the index of the element being processed when the panic
	// occurred. It is -1 for operations without element indices
	// (Join, Scope).
	Index int
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("par: worker panic at index %d: %v", e.Index, e.Value)
}

// Unwrap returns Value if it is itself an error, so errors.Is/As chains
// reach through a panicked error value. Otherwise it returns nil.
func (e *PanicError) Unwrap() error {
	if err, ok := e.Value.(error); ok {
		return err
	}
	return nil
}
