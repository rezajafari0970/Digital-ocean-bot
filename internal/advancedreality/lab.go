package advancedreality

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
)

var ErrConflict = errors.New("lab_state_conflict")
var ErrStorage = errors.New("lab_storage_error")
var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// Lab is a private, explicitly initialized snapshot store. It does not write
// live Xray/panel files or execute/restart processes. State is one atomic file
// so a crash cannot separate the configuration from its rollback metadata.
type Lab struct{ Dir string }
type labState struct {
	Owner      string               `json:"owner"`
	Core       string               `json:"core"`
	Baseline   []byte               `json:"baseline"`
	Current    []byte               `json:"current"`
	Revision   int                  `json:"revision"`
	Operations map[string]operation `json:"operations"`
}
type operation struct {
	Request string  `json:"request"`
	Receipt Receipt `json:"receipt"`
}
type Receipt struct {
	Revision          int    `json:"revision"`
	CurrentSHA256     string `json:"current_sha256"`
	BaselineSHA256    string `json:"baseline_sha256"`
	ProductionChanged bool   `json:"production_changed"`
}

func (l Lab) Init(raw []byte, core string) (Receipt, error) {
	if _, e := Inspect(raw, raw, core); e != nil {
		return Receipt{}, e
	}
	if !filepath.IsAbs(l.Dir) || filepath.Clean(l.Dir) == "/" {
		return Receipt{}, ErrStorage
	}
	if e := os.Mkdir(l.Dir, 0700); e != nil {
		return Receipt{}, ErrStorage
	}
	// Only files inside this newly created directory belong to the experiment.
	f, e := os.OpenFile(filepath.Join(l.Dir, "lock"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		return Receipt{}, ErrStorage
	}
	defer f.Close()
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		return Receipt{}, ErrStorage
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	s := labState{Owner: "dob-advanced-reality-lab-v1", Core: core, Baseline: raw, Current: raw, Revision: 1, Operations: map[string]operation{}}
	if e = l.save(s); e != nil {
		return Receipt{}, e
	}
	return receipt(s), nil
}
func receipt(s labState) Receipt {
	return Receipt{Revision: s.Revision, CurrentSHA256: Digest(s.Current), BaselineSHA256: Digest(s.Baseline)}
}

func (l Lab) locked(fn func(labState) (Receipt, error)) (Receipt, error) {
	info, e := os.Lstat(l.Dir)
	if e != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return Receipt{}, ErrStorage
	}
	f, e := os.OpenFile(filepath.Join(l.Dir, "lock"), os.O_RDWR|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return Receipt{}, ErrStorage
	}
	defer f.Close()
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		return Receipt{}, ErrStorage
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	fi, e := f.Stat()
	pi, pe := os.Lstat(filepath.Join(l.Dir, "lock"))
	if e != nil || pe != nil || !os.SameFile(fi, pi) {
		return Receipt{}, ErrStorage
	}
	f2, e := os.OpenFile(filepath.Join(l.Dir, "state.json"), os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return Receipt{}, ErrStorage
	}
	defer f2.Close()
	info, e = f2.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > 3*MaxConfigBytes {
		return Receipt{}, ErrStorage
	}
	var s labState
	d := json.NewDecoder(f2)
	d.DisallowUnknownFields()
	if d.Decode(&s) != nil || d.Decode(new(any)) != io.EOF || s.Owner != "dob-advanced-reality-lab-v1" || s.Revision < 1 || s.Operations == nil || len(s.Operations) > 64 {
		return Receipt{}, ErrStorage
	}
	if _, e = Inspect(s.Baseline, s.Current, s.Core); e != nil {
		return Receipt{}, ErrStorage
	}
	return fn(s)
}

func (l Lab) save(s labState) error {
	b, e := json.Marshal(s)
	if e != nil {
		return ErrStorage
	}
	f, e := os.CreateTemp(l.Dir, "state.pending-")
	if e != nil {
		return ErrStorage
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil || ce != nil {
		return ErrStorage
	}
	if e = os.Rename(name, filepath.Join(l.Dir, "state.json")); e != nil {
		return ErrStorage
	}
	d, e := os.Open(l.Dir)
	if e != nil {
		return ErrStorage
	}
	defer d.Close()
	if d.Sync() != nil {
		return ErrStorage
	}
	return nil
}

func (l Lab) Status() (Receipt, error) {
	return l.locked(func(s labState) (Receipt, error) { return receipt(s), nil })
}

// CurrentConfig explicitly exports sensitive bytes; callers must keep them in
// private files and must never include them in logs or telemetry.
func (l Lab) CurrentConfig() ([]byte, error) {
	var raw []byte
	_, e := l.locked(func(s labState) (Receipt, error) { raw = append([]byte(nil), s.Current...); return receipt(s), nil })
	return raw, e
}

// Stage and Rollback are CAS operations on this lab's snapshot only. Request
// keys are durable; reusing a key for a different request is always rejected.
func (l Lab) Stage(raw []byte, expected, key string) (Receipt, error) {
	return l.change(raw, expected, key, false)
}
func (l Lab) Rollback(expected, key string) (Receipt, error) {
	return l.change(nil, expected, key, true)
}
func (l Lab) change(raw []byte, expected, key string, rollback bool) (Receipt, error) {
	if !keyPattern.MatchString(key) {
		return Receipt{}, ErrConflict
	}
	return l.locked(func(s labState) (Receipt, error) {
		action := "stage"
		if rollback {
			action = "rollback"
			raw = s.Baseline
		}
		request := Digest([]byte(action + "\x00" + expected + "\x00" + Digest(raw)))
		if prev, ok := s.Operations[key]; ok {
			if prev.Request != request {
				return Receipt{}, ErrConflict
			}
			return prev.Receipt, nil
		}
		if len(s.Operations) >= 64 || Digest(s.Current) != expected {
			return Receipt{}, ErrConflict
		}
		if _, e := Inspect(s.Baseline, raw, s.Core); e != nil {
			return Receipt{}, e
		}
		s.Current = append([]byte(nil), raw...)
		s.Revision++
		s.Operations[key] = operation{Request: request, Receipt: receipt(s)}
		if e := l.save(s); e != nil {
			return Receipt{}, e
		}
		return receipt(s), nil
	})
}

// Remove only accepts an exact, restored, owned lab. Unknown files, changed
// state and symlinks block removal. It never recursively removes a directory.
func (l Lab) Remove(expected string) error {
	_, e := l.locked(func(s labState) (Receipt, error) {
		if Digest(s.Current) != expected || Digest(s.Current) != Digest(s.Baseline) {
			return Receipt{}, ErrConflict
		}
		entries, e := os.ReadDir(l.Dir)
		if e != nil || len(entries) != 2 {
			return Receipt{}, ErrStorage
		}
		for _, x := range entries {
			if x.Name() != "lock" && x.Name() != "state.json" {
				return Receipt{}, ErrStorage
			}
		}
		if os.Remove(filepath.Join(l.Dir, "state.json")) != nil {
			return Receipt{}, ErrStorage
		}
		if os.Remove(filepath.Join(l.Dir, "lock")) != nil {
			return Receipt{}, ErrStorage
		}
		if os.Remove(l.Dir) != nil {
			return Receipt{}, ErrStorage
		}
		return receipt(s), nil
	})
	return e
}
