package discovm

import (
	"errors"
	"syscall"
)

// wsaeConnRefused is Winsock's WSAECONNREFUSED, which a refused dial reports
// on Windows rather than syscall.ECONNREFUSED.
const wsaeConnRefused = syscall.Errno(10061)

// connectionRefused reports a dial the far host answered by refusing it:
// the host was reached, and nothing listens on the port.
func connectionRefused(err error) bool { return errors.Is(err, wsaeConnRefused) }
