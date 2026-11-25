package traefiktest

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive advisory lock, blocking until it is free. The
// kernel drops it if the process dies, so a crashed test run cannot wedge the cache.
func lockFile(name string) (unlock func(), err error) {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil // closing releases the lock
}
