package serverprotection

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type processHealth struct {
	fdPercent float64
	fdKnown   bool
	xrayPIDs  []string
}

func managedProcesses(root string) (processHealth, error) {
	var h processHealth
	entries, err := os.ReadDir(root)
	if err != nil {
		return h, err
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		exe, err := os.Readlink(filepath.Join(dir, "exe"))
		if err != nil {
			if os.IsPermission(err) {
				return h, fmt.Errorf("process inventory permission denied: %w", err)
			}
			continue
		}
		exe = strings.TrimSuffix(exe, " (deleted)")
		xray := strings.HasPrefix(exe, "/usr/local/x-ui/bin/xray-linux-")
		if !xray && exe != "/usr/local/x-ui/x-ui" {
			continue
		}
		if xray {
			h.xrayPIDs = append(h.xrayPIDs, entry.Name())
		}
		limits, err := os.ReadFile(filepath.Join(dir, "limits"))
		if err != nil {
			continue
		}
		fds, err := os.ReadDir(filepath.Join(dir, "fd"))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(limits), "\n") {
			if !strings.HasPrefix(line, "Max open files") {
				continue
			}
			f := strings.Fields(line)
			if len(f) < 4 {
				continue
			}
			max, err := strconv.ParseFloat(f[3], 64)
			if err != nil || max <= 0 {
				continue
			}
			h.fdKnown = true
			h.fdPercent = math.Max(h.fdPercent, ratio(float64(len(fds)), max))
		}
	}
	return h, nil
}

// Require listeners owned by a managed Xray process, not just any process
// occupying the requested port. No synthetic connection consumes admission.
func xrayListeners(root string, ports []int) (bool, error) {
	h, err := managedProcesses(root)
	if err != nil {
		return false, err
	}
	if len(h.xrayPIDs) == 0 {
		return false, nil
	}
	sockets := map[string]bool{}
	for _, pid := range h.xrayPIDs {
		dir := filepath.Join(root, pid, "fd")
		fds, err := os.ReadDir(dir)
		if err != nil {
			return false, err
		}
		for _, fd := range fds {
			target, e := os.Readlink(filepath.Join(dir, fd.Name()))
			if e == nil && strings.HasPrefix(target, "socket:[") {
				sockets[strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")] = true
			}
		}
	}
	listening := map[int]bool{}
	for _, netfile := range []string{"tcp", "tcp6"} {
		raw, e := os.ReadFile(filepath.Join(root, "net", netfile))
		if e != nil {
			if netfile == "tcp6" && os.IsNotExist(e) {
				continue
			}
			return false, e
		}
		for _, line := range strings.Split(string(raw), "\n") {
			f := strings.Fields(line)
			if len(f) < 10 || f[3] != "0A" || !sockets[f[9]] {
				continue
			}
			_, p, ok := strings.Cut(f[1], ":")
			if !ok {
				continue
			}
			n, e := strconv.ParseInt(p, 16, 32)
			if e == nil {
				listening[int(n)] = true
			}
		}
	}
	if len(ports) == 0 {
		return false, fmt.Errorf("no managed ports")
	}
	for _, p := range ports {
		if !listening[p] {
			return false, nil
		}
	}
	return true, nil
}
