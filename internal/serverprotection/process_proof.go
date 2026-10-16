package serverprotection

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// processIdentity binds a saved sample to its writer and executing inode.
// Start ticks, unlike a PID alone, distinguish reused process IDs.
type processIdentity struct {
	PID        int    `json:"producer_pid"`
	StartTicks string `json:"producer_start_ticks"`
	SHA256     string `json:"producer_sha256"`
}

func processStart(root string, pid int) (string, error) {
	if pid <= 0 {
		return "", fmt.Errorf("invalid guardian producer")
	}
	raw, err := os.ReadFile(filepath.Join(root, strconv.Itoa(pid), "stat"))
	if err != nil {
		return "", err
	}
	_, rest, ok := strings.Cut(string(raw), ")")
	// comm may contain ')'; use its final delimiter.
	if i := strings.LastIndexByte(string(raw), ')'); i >= 0 {
		rest = string(raw)[i+1:]
		ok = true
	}
	fields := strings.Fields(rest)
	if !ok || len(fields) <= 19 {
		return "", fmt.Errorf("invalid guardian process stat")
	}
	if _, err = strconv.ParseUint(fields[19], 10, 64); err != nil {
		return "", fmt.Errorf("invalid guardian start time")
	}
	return fields[19], nil
}
func observeProcess(root string, pid int) (processIdentity, error) {
	result := processIdentity{PID: pid}
	start, err := processStart(root, pid)
	if err != nil {
		return result, err
	}
	path := filepath.Join(root, strconv.Itoa(pid), "exe")
	// Open the kernel executable link, pinning the executing inode across rename.
	file, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return result, err
	}
	h := sha256.New()
	if _, err = io.Copy(h, file); err != nil {
		return result, err
	}
	after, err := os.Stat(path)
	if err != nil {
		return result, err
	}
	finalStart, err := processStart(root, pid)
	if err != nil {
		return result, err
	}
	if start != finalStart || !os.SameFile(before, after) {
		return result, fmt.Errorf("guardian process changed during observation")
	}
	result.StartTicks = start
	result.SHA256 = hex.EncodeToString(h.Sum(nil))
	return result, nil
}

var selfIdentity struct {
	sync.Once
	identity processIdentity
	err      error
}

func writerIdentity() (processIdentity, error) {
	selfIdentity.Do(func() { selfIdentity.identity, selfIdentity.err = observeProcess("/proc", os.Getpid()) })
	return selfIdentity.identity, selfIdentity.err
}
func verifyProducer(saved processIdentity) (processIdentity, error) {
	if saved.PID <= 0 || saved.StartTicks == "" || saved.SHA256 == "" {
		return processIdentity{}, fmt.Errorf("guardian sample producer absent")
	}
	current, err := observeProcess("/proc", saved.PID)
	if err != nil {
		return processIdentity{}, err
	}
	if current != saved {
		return processIdentity{}, fmt.Errorf("guardian sample producer changed")
	}
	return current, nil
}
