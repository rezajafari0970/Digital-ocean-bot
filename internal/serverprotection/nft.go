package serverprotection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

const nftOwner = "digital-ocean-bot server-protection v1"

type NFT struct{ Path string }

func runCommand(ctx context.Context, name string, args []string, input string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, 2e9)
	defer cancel()
	c := exec.CommandContext(cctx, name, args...)
	c.Env = append(os.Environ(), "LC_ALL=C")
	if input != "" {
		c.Stdin = strings.NewReader(input)
	}
	out, e := c.CombinedOutput()
	if e != nil {
		return out, fmt.Errorf("%s failed: %w: %.300s", name, e, out)
	}
	return out, nil
}
func (n NFT) present(ctx context.Context) (bool, error) {
	_, present, e := n.ownedTable(ctx)
	return present, e
}
func (n NFT) ownedTable(ctx context.Context) (uint64, bool, error) {
	b, e := runCommand(ctx, n.Path, []string{"-a", "-j", "list", "table", "inet", "dob_guardian"}, "")
	if e != nil {
		if strings.Contains(string(b), "No such file or directory") {
			return 0, false, nil
		}
		return 0, false, e
	}
	var v struct {
		NFTables []struct {
			Table *struct {
				Name, Family, Comment string
				Handle                uint64
			} `json:"table"`
		} `json:"nftables"`
	}
	if json.Unmarshal(b, &v) != nil {
		return 0, false, errors.New("invalid nft readback")
	}
	for _, x := range v.NFTables {
		if x.Table != nil && x.Table.Name == "dob_guardian" && x.Table.Family == "inet" {
			if x.Table.Comment != nftOwner {
				return 0, false, errors.New("foreign dob_guardian table; not modified")
			}
			if x.Table.Handle == 0 {
				return 0, false, errors.New("nft table handle missing")
			}
			return x.Table.Handle, true, nil
		}
	}
	return 0, false, errors.New("nft table ownership not verified")
}
func (n NFT) Apply(ctx context.Context, ports []int, block bool) error {
	handle, present, err := n.ownedTable(ctx)
	if err != nil {
		return err
	}
	var values []string
	if block && len(ports) == 0 {
		return errors.New("no eligible VLESS ports")
	}
	for _, p := range ports {
		if p < 1 || p > 65535 || p == 22 {
			return errors.New("unsafe admission port")
		}
		if block {
			values = append(values, strconv.Itoa(p)+" timeout 15s")
		}
	}
	// One nft transaction restores only our owned table and changes admission
	// atomically. Kernel success proves the rule, chain and set exist together.
	script := ""
	if present {
		script = fmt.Sprintf("delete table inet handle %d\n", handle)
	}
	script += `add table inet dob_guardian {
 comment "digital-ocean-bot server-protection v1"
 set blocked_ports { type inet_service; flags timeout; timeout 15s; }
 chain admission {
  type filter hook input priority -5; policy accept;
  iifname "lo" return
  ct state new tcp flags & (syn | ack) == syn tcp dport @blocked_ports counter reject with tcp reset
 }
}
`
	if block {
		script += "add element inet dob_guardian blocked_ports { " + strings.Join(values, ", ") + " }\n"
	}
	_, err = runCommand(ctx, n.Path, []string{"-f", "-"}, script)
	return err
}
func (n NFT) Remove(ctx context.Context) error {
	handle, yes, e := n.ownedTable(ctx)
	if e != nil || !yes {
		return e
	}
	_, e = runCommand(ctx, n.Path, []string{"delete", "table", "inet", "handle", strconv.FormatUint(handle, 10)}, "")
	return e
}
func ParsePorts(r io.Reader, excluded []int) ([]int, error) {
	var c struct {
		Inbounds []struct {
			Protocol string          `json:"protocol"`
			Port     json.RawMessage `json:"port"`
			Listen   string          `json:"listen"`
			Stream   struct {
				Network string `json:"network"`
			} `json:"streamSettings"`
		} `json:"inbounds"`
	}
	dec := json.NewDecoder(io.LimitReader(r, 32<<20))
	if e := dec.Decode(&c); e != nil {
		return nil, errors.New("cannot parse current Xray configuration")
	}
	blocked := map[int]bool{22: true}
	for _, p := range excluded {
		blocked[p] = true
	}
	seen := map[int]bool{}
	for _, i := range c.Inbounds {
		if i.Protocol != "vless" || (i.Stream.Network != "" && i.Stream.Network != "tcp" && i.Stream.Network != "raw") {
			continue
		}
		if i.Listen != "" {
			ip := net.ParseIP(i.Listen)
			if ip == nil || ip.IsLoopback() {
				continue
			}
		}
		var port int
		if json.Unmarshal(i.Port, &port) != nil {
			var str string
			if json.Unmarshal(i.Port, &str) != nil {
				continue
			}
			port, _ = strconv.Atoi(str)
		}
		if port > 0 && port <= 65535 && !blocked[port] {
			seen[port] = true
		}
	}
	ports := []int{}
	for p := range seen {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	if len(ports) == 0 {
		return ports, errors.New("no eligible public VLESS TCP ports")
	}
	return ports, nil
}
func LivePorts(p Policy) ([]int, error) {
	f, e := os.Open(XrayConfig)
	if e != nil {
		return nil, errors.New("running Xray config unavailable")
	}
	defer f.Close()
	return ParsePorts(f, p.ExcludedPorts)
}
