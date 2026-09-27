package listeners

import (
	"bufio"
	"strconv"
	"strings"
)

func ParseSS(s string) []int {
	seen := map[int]bool{}
	out := []int{}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 4 {
			continue
		}
		addr := f[3]
		i := strings.LastIndex(addr, ":")
		if i < 0 {
			continue
		}
		p, e := strconv.Atoi(strings.Trim(addr[i+1:], "[]"))
		if e == nil && p > 0 && p <= 65535 && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}
