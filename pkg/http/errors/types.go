package errors

// BadRequest indicates that the server could not understand the request for whatever reason.
type BadRequest struct {
	Err error
}

// Error is BadRequest implementing the error interface.
func (e *BadRequest) Error() string {
	return e.Err.Error()
}