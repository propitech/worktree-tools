package slot

import (
	"os"
	"syscall"
)

// Lock serialises slot allocation across concurrent add/adopt calls for the
// same repo. Use Acquire then defer Release.
type Lock struct{ f *os.File }

// Acquire opens lockPath and takes an exclusive flock. Returns an error if the
// file cannot be created or the lock fails.
func (l *Lock) Acquire(lockPath string) error {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return err
	}
	l.f = f
	return nil
}

// Release unlocks and closes the lock file. Safe to call multiple times.
func (l *Lock) Release() {
	if l.f != nil {
		_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
		l.f.Close()
		l.f = nil
	}
}
