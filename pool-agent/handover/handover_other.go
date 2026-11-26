//go:build !unix

package handover

import (
	"context"
	"net"
	"os"
	"sync"
)

// Where there is no flock(2), only this process's own locks exclude each
// other: no other process of a pool hands over there.
var (
	localMu    sync.Mutex
	localLocks = map[string]int{} // path: -1 held alone, n > 0 held by n marks
	localFiles = map[*os.File]string{}
)

func tryLock(file *os.File, exclusive bool) (bool, error) {
	localMu.Lock()
	defer localMu.Unlock()
	path := file.Name()
	held := localLocks[path]
	switch {
	case exclusive && held != 0, !exclusive && held < 0:
		return false, nil
	case exclusive:
		localLocks[path] = -1
	default:
		localLocks[path] = held + 1
	}
	localFiles[file] = path
	return true, nil
}

func unlock(file *os.File) {
	localMu.Lock()
	defer localMu.Unlock()
	path, ok := localFiles[file]
	if !ok {
		return
	}
	delete(localFiles, file)
	if localLocks[path] <= 1 {
		delete(localLocks, path)
		return
	}
	localLocks[path]--
}

// DrainRequested is never closed where no pool's units can ask for a drain.
func DrainRequested(context.Context) <-chan struct{} { return make(chan struct{}) }

// Listeners are none where systemd passes none.
func Listeners() (map[string]net.Listener, error) { return map[string]net.Listener{}, nil }
