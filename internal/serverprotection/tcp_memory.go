package serverprotection

import (
	"strconv"
	"strings"
)

// tcpMemory reads kernel page counters, not byte counters. Invalid or absent
// data stays unknown; it must never masquerade as a zero-pressure sample.
func tcpMemory(sockstat, limits []byte, pageSize int) (used, pressure, maximum float64, ok bool) {
	if pageSize <= 0 {
		return
	}
	f := strings.Fields(string(limits))
	if len(f) != 3 {
		return
	}
	var budget [3]uint64
	for i, s := range f {
		n, e := strconv.ParseUint(s, 10, 64)
		if e != nil || n > 1<<53 {
			return
		}
		budget[i] = n
	}
	if budget[1] == 0 || budget[2] == 0 || budget[0] > budget[1] || budget[1] > budget[2] {
		return
	}
	var pages uint64
	found := false
	for _, line := range strings.Split(string(sockstat), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "TCP:" {
			continue
		}
		if len(fields)%2 != 1 {
			return
		}
		for i := 1; i+1 < len(fields); i += 2 {
			if fields[i] != "mem" {
				continue
			}
			if found {
				return
			}
			n, e := strconv.ParseUint(fields[i+1], 10, 64)
			if e != nil || n > 1<<53 {
				return
			}
			pages = n
			found = true
		}
	}
	if !found {
		return
	}
	scale := float64(pageSize) / (1024 * 1024)
	return float64(pages) * scale, float64(budget[1]) * scale, float64(budget[2]) * scale, true
}
