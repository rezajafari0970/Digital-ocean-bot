package kamatera

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/url"
	"regexp"
)

// Suggestions are controller presets, not provider SKUs or price quotations.
// Every dimension and traffic package must occur in this region's live response.
type capabilityData struct {
	Disks          []int             `json:"diskSizeGB"`
	Traffic        map[string]string `json:"monthlyTrafficPackage"`
	DefaultTraffic string            `json:"defaultMonthlyTrafficPackage"`
	CPUs           []struct {
		ID    string `json:"id"`
		Cores []int  `json:"cpuCores"`
		RAM   []int  `json:"ramMB"`
	} `json:"cpuTypes"`
}

var trafficPackagePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func containsDimension(xs []int, want int) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
func (d *Driver) suggestedSizes(ctx context.Context, region string) ([]sizeData, error) {
	var c capabilityData
	if e := d.client.do(ctx, "GET", "/service/server?capabilities=1&datacenter="+url.QueryEscape(region), nil, &c); e != nil {
		return nil, e
	}
	if len(c.CPUs) == 0 || len(c.CPUs) > 8 || len(c.Disks) == 0 || len(c.Disks) > 256 ||
		!trafficPackagePattern.MatchString(c.DefaultTraffic) || c.Traffic[c.DefaultTraffic] == "" {
		return nil, problem(providers.ErrorUnavailable, "catalog", "INVALID_CAPABILITIES", 200)
	}
	// At most 32 suggestions; retain all predefined sizes independently.
	shapes := [][3]int{{1, 1024, 20}, {1, 2048, 20}, {2, 2048, 20}, {2, 4096, 30}, {4, 8192, 60}, {8, 16384, 100}, {16, 32768, 150}, {32, 65536, 200}}
	out := []sizeData{}
	seen := map[string]bool{}
	for _, cpu := range c.CPUs {
		if seen[cpu.ID] || len(cpu.Cores) == 0 || len(cpu.Cores) > 256 || len(cpu.RAM) == 0 || len(cpu.RAM) > 256 {
			return nil, problem(providers.ErrorUnavailable, "catalog", "INVALID_CAPABILITIES", 200)
		}
		seen[cpu.ID] = true
		for _, shape := range shapes {
			s := sizeData{Suggested: true, Cores: shape[0], RAM: shape[1], Disk: shape[2], CPUType: cpu.ID, Traffic: c.DefaultTraffic}
			if s.valid() && containsDimension(cpu.Cores, s.Cores) && containsDimension(cpu.RAM, s.RAM) && containsDimension(c.Disks, s.Disk) {
				out = append(out, s)
			}
		}
	}
	return out, nil
}
func supportedSize(s sizeData, images []imageData) bool {
	for _, im := range images {
		if im.version() != "" && im.Disk > 0 && s.Disk >= im.Disk && im.RAM[s.CPUType] > 0 && s.RAM >= im.RAM[s.CPUType] {
			return true
		}
	}
	return false
}
