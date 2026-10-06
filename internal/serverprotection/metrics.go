package serverprotection

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Sampler struct {
	previous map[string]float64
	at       time.Time
}

func number(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }
func ratio(n, d float64) float64 {
	if d <= 0 {
		return 0
	}
	return math.Max(0, math.Min(100, n/d*100))
}
func readNumber(path string) (float64, bool) {
	b, e := os.ReadFile(path)
	if e != nil {
		return 0, false
	}
	n, e := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
	return n, e == nil
}
func (s *Sampler) Read(now time.Time) (Metrics, error) {
	m := Metrics{InodeFreePercent: 100}
	b, e := os.ReadFile("/proc/meminfo")
	if e != nil {
		return m, e
	}
	mem := map[string]float64{}
	for _, line := range strings.Split(string(b), "\n") {
		v := strings.Fields(line)
		if len(v) >= 2 {
			mem[strings.TrimSuffix(v[0], ":")] = number(v[1]) / 1024
		}
	}
	m.MemTotalMB = mem["MemTotal"]
	m.MemAvailableMB = mem["MemAvailable"]
	m.SwapUsedMB = mem["SwapTotal"] - mem["SwapFree"]
	if m.MemTotalMB <= 0 || mem["MemAvailable"] < 0 {
		return m, fmt.Errorf("memory counters unavailable")
	}
	if _, ok := mem["MemAvailable"]; !ok {
		return m, fmt.Errorf("MemAvailable unavailable")
	}
	cur := map[string]float64{}
	elapsed := now.Sub(s.at).Seconds()
	delta := func(k string, v float64) float64 {
		cur[k] = v
		if s.at.IsZero() || elapsed <= 0 {
			return 0
		}
		old, ok := s.previous[k]
		if !ok || v < old {
			return 0
		}
		return v - old
	}
	b, e = os.ReadFile("/proc/stat")
	if e != nil {
		return m, e
	}
	for _, line := range strings.Split(string(b), "\n") {
		v := strings.Fields(line)
		if len(v) < 8 || !strings.HasPrefix(v[0], "cpu") {
			continue
		}
		total := float64(0)
		for i := 1; i < len(v) && i <= 8; i++ {
			total += number(v[i])
		}
		idle := number(v[4]) + number(v[5])
		steal := float64(0)
		if len(v) > 8 {
			steal = number(v[8])
		}
		dt := delta(v[0]+"total", total)
		di := delta(v[0]+"idle", idle)
		ds := delta(v[0]+"steal", steal)
		usage := ratio(dt-di-ds, dt)
		if v[0] == "cpu" {
			m.CPUPercent = usage
			m.CPUStealPercent = ratio(ds, dt)
		} else {
			m.CPUHotCorePercent = math.Max(m.CPUHotCorePercent, usage)
		}
	}
	for _, kind := range []string{"memory", "cpu", "io"} {
		b, e = os.ReadFile("/proc/pressure/" + kind)
		if e != nil {
			continue
		}
		m.PSISupported = true
		for _, line := range strings.Split(string(b), "\n") {
			target := "full "
			if kind == "cpu" {
				target = "some "
			}
			if !strings.HasPrefix(line, target) {
				continue
			}
			for _, f := range strings.Fields(line) {
				if strings.HasPrefix(f, "total=") {
					d := delta("psi-"+kind, number(strings.TrimPrefix(f, "total=")))
					v := ratio(d, elapsed*1e6)
					switch kind {
					case "memory":
						m.MemoryPSI = v
					case "cpu":
						m.CPUPSI = v
					case "io":
						m.IOPSI = v
					}
				}
			}
		}
	}
	if b, e = os.ReadFile("/proc/sys/fs/file-nr"); e == nil {
		f := strings.Fields(string(b))
		if len(f) == 3 {
			m.FDPercent = ratio(number(f[0])-number(f[1]), number(f[2]))
		}
	}
	count, ok := readNumber("/proc/sys/net/netfilter/nf_conntrack_count")
	max, ok2 := readNumber("/proc/sys/net/netfilter/nf_conntrack_max")
	m.ConntrackSupported = ok && ok2 && max > 0
	if m.ConntrackSupported {
		m.ConntrackPercent = ratio(count, max)
	}
	var fs syscall.Statfs_t
	if e = syscall.Statfs("/", &fs); e != nil {
		return m, e
	}
	m.DiskFreeMB = float64(fs.Bavail) * float64(fs.Bsize) / (1024 * 1024)
	m.DiskFreePercent = ratio(float64(fs.Bavail), float64(fs.Blocks))
	if fs.Files > 0 {
		m.InodeFreePercent = ratio(float64(fs.Ffree), float64(fs.Files))
	}
	if b, e = os.ReadFile("/proc/net/dev"); e == nil {
		var rx, tx, packets, drops float64
		for _, line := range strings.Split(string(b), "\n") {
			iface, rest, ok := strings.Cut(line, ":")
			if !ok || strings.TrimSpace(iface) == "lo" {
				continue
			}
			f := strings.Fields(rest)
			if len(f) >= 16 {
				rx += number(f[0])
				tx += number(f[8])
				packets += number(f[1]) + number(f[9])
				drops += number(f[3]) + number(f[11])
			}
		}
		drx, dtx, dp, dd := delta("rx", rx), delta("tx", tx), delta("packets", packets), delta("drops", drops)
		if elapsed > 0 && !s.at.IsZero() {
			m.RXMbps = drx * 8 / elapsed / 1e6
			m.TXMbps = dtx * 8 / elapsed / 1e6
			m.PacketsPerSecond = dp / elapsed
			m.DroppedPerSecond = dd / elapsed
		}
	}
	s.previous = cur
	s.at = now
	return m, nil
}
