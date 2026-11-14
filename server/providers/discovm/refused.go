//go:build !windows

package discovm

import (
	"errors"
	"syscall"
)

// connectionRefused reports a dial the far host answered by refusing it:
// the host was reached, and nothing listens on the port.
func connectionRefused(err error) bool { return errors.Is(err, syscall.ECONNREFUSED) }
