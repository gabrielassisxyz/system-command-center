package server

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// OsProcessKiller is the production ProcessKiller. It sends SIGTERM to the
// process identified by pid. Errors are wrapped so the HTTP handler can map
// them to status codes.
type OsProcessKiller struct{}

// Kill sends SIGTERM to pid.
func (OsProcessKiller) Kill(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("%w: find process %d: %v", ErrProcessNotFound, pid, err)
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) || errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("%w: %v", ErrProcessNotFound, err)
		}
		return err
	}
	return nil
}
