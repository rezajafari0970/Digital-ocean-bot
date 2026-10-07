package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ProcessRoleLock complements SQL ownership on the supported single worker host.
// Losing a SQL session cannot release this lifetime fence, including all/split
// transitions. Multi-host active/active operation needs a separate fencing design.
type ProcessRoleLock struct{ files []*os.File }

func AcquireProcessRole(root string, role Role) (*ProcessRoleLock, error) {
	if _, err := ParseRole(string(role)); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	l := &ProcessRoleLock{}
	ok := false
	defer func() {
		if !ok {
			l.Close()
		}
	}()
	for _, group := range []Role{RoleControl, RolePanels} {
		if !role.Owns(group) {
			continue
		}
		f, err := os.OpenFile(filepath.Join(root, string(group)+".lock"), os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			f.Close()
			return nil, fmt.Errorf("%w: process %s", ErrRoleOwned, group)
		}
		l.files = append(l.files, f)
	}
	ok = true
	return l, nil
}
func (l *ProcessRoleLock) Close() {
	if l == nil {
		return
	}
	for _, f := range l.files {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}
	l.files = nil
}
