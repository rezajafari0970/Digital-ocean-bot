package aws

import (
	"context"
	"net/url"
	"sort"
	"strings"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

const canonicalOwner = "099720109477"

type image struct {
	ID             string `xml:"imageId"`
	Name           string `xml:"name"`
	Owner          string `xml:"imageOwnerId"`
	State          string `xml:"imageState"`
	Architecture   string `xml:"architecture"`
	Root           string `xml:"rootDeviceName"`
	RootType       string `xml:"rootDeviceType"`
	Virtualization string `xml:"virtualizationType"`
	Created        string `xml:"creationDate"`
	Public         bool   `xml:"isPublic"`
	Products       []struct {
		ID string `xml:"productCode"`
	} `xml:"productCodes>item"`
	Devices []struct {
		Device string `xml:"deviceName"`
		EBS    struct {
			Size int `xml:"volumeSize"`
		} `xml:"ebs"`
	} `xml:"blockDeviceMapping>item"`
}

func (x image) RootSize() int {
	for _, b := range x.Devices {
		if b.Device == x.Root {
			return b.EBS.Size
		}
	}
	return 0
}
func imageVersion(name string) string {
	// Official Canonical public amd64 server images only; excludes Pro, minimal,
	// marketplace and arbitrary images whose name merely includes "Ubuntu".
	for _, v := range []string{"26.04", "24.04", "22.04"} {
		if strings.HasPrefix(name, "ubuntu/images/hvm-ssd/") || strings.HasPrefix(name, "ubuntu/images/hvm-ssd-gp3/") {
			if strings.Contains(name, "-"+v+"-amd64-server-") {
				return v
			}
		}
	}
	return ""
}
func usableImage(x image) bool {
	return idPattern.MatchString(x.ID) && strings.HasPrefix(x.ID, "ami-") && x.Owner == canonicalOwner && x.Public && x.State == "available" && x.Architecture == "x86_64" && x.RootType == "ebs" && x.Virtualization == "hvm" && x.Root != "" && x.RootSize() > 0 && len(x.Devices) == 1 && len(x.Products) == 0 && imageVersion(x.Name) != ""
}
func (d *Driver) images(ctx context.Context, q url.Values) ([]image, error) {
	if q == nil {
		q = url.Values{}
	}
	if id := q.Get("ImageId.1"); id != "" {
		q.Del("ImageId.1")
		q.Set("Filter.5.Name", "image-id")
		q.Set("Filter.5.Value.1", id)
	}
	q.Set("Owner.1", canonicalOwner)
	q.Set("Filter.1.Name", "state")
	q.Set("Filter.1.Value.1", "available")
	q.Set("Filter.2.Name", "architecture")
	q.Set("Filter.2.Value.1", "x86_64")
	q.Set("Filter.3.Name", "root-device-type")
	q.Set("Filter.3.Value.1", "ebs")
	q.Set("Filter.4.Name", "name")
	q.Set("Filter.4.Value.1", "ubuntu/images/hvm-ssd*/ubuntu-*-amd64-server-*")
	return pages(ctx, d.client, "DescribeImages", "imagesSet", q, func(x image) string { return x.ID })
}

type instanceType struct {
	ID  string `xml:"instanceType"`
	CPU struct {
		Count int `xml:"defaultVCpus"`
	} `xml:"vCpuInfo"`
	Memory struct {
		MB int `xml:"sizeInMiB"`
	} `xml:"memoryInfo"`
	Processor struct {
		Architectures []string `xml:"supportedArchitectures>item"`
	} `xml:"processorInfo"`
	Hypervisor string   `xml:"hypervisor"`
	Usage      []string `xml:"supportedUsageClasses>item"`
}

func (d *Driver) plans(ctx context.Context, selected string) ([]providers.Plan, error) {
	q := url.Values{"Filter.1.Name": {"processor-info.supported-architecture"}, "Filter.1.Value.1": {"x86_64"}, "Filter.2.Name": {"supported-usage-class"}, "Filter.2.Value.1": {"on-demand"}}
	if selected != "" {
		q.Set("Filter.3.Name", "instance-type")
		q.Set("Filter.3.Value.1", selected)
	}
	xs, e := pages(ctx, d.client, "DescribeInstanceTypes", "instanceTypeSet", q, func(x instanceType) string { return x.ID })
	if e != nil {
		return nil, e
	}
	offers, e := pages(ctx, d.client, "DescribeInstanceTypeOfferings", "instanceTypeOfferingSet", url.Values{"LocationType": {"region"}, "Filter.1.Name": {"location"}, "Filter.1.Value.1": {d.client.region}}, func(x struct {
		ID       string `xml:"instanceType"`
		Location string `xml:"location"`
	}) string { return x.ID })
	if e != nil {
		return nil, e
	}
	available := map[string]bool{}
	for _, o := range offers {
		if o.Location != d.client.region {
			return nil, problem(providers.ErrorUnavailable, "catalog", "REGION_MISMATCH", 200)
		}
		available[o.ID] = true
	}
	out := []providers.Plan{}
	for _, x := range xs {
		if available[x.ID] && x.CPU.Count > 0 && x.Memory.MB > 0 && contains(x.Processor.Architectures, "x86_64") && contains(x.Usage, "on-demand") {
			out = append(out, providers.Plan{ID: x.ID, Name: x.ID, CPU: x.CPU.Count, MemoryMB: x.Memory.MB, DiskGB: 12, Available: true, AvailableRegions: []string{d.client.region}})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MemoryMB != out[j].MemoryMB {
			return out[i].MemoryMB < out[j].MemoryMB
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
func (d *Driver) Catalog(ctx context.Context) (providers.Catalog, error) {
	cred, e := credentials(ctx, d.client.credentials)
	if e != nil {
		return providers.Catalog{}, e
	}
	if e = d.validateNetwork(ctx, cred); e != nil {
		return providers.Catalog{}, e
	}
	ps, e := d.plans(ctx, "")
	if e != nil {
		return providers.Catalog{}, e
	}
	xs, e := d.images(ctx, nil)
	if e != nil {
		return providers.Catalog{}, e
	}
	latest := map[string]image{}
	for _, im := range xs {
		if usableImage(im) {
			v := imageVersion(im.Name)
			if im.Created > latest[v].Created {
				latest[v] = im
			}
		}
	}
	images := []providers.Image{}
	for _, v := range []string{"26.04", "24.04", "22.04"} {
		if x, ok := latest[v]; ok {
			images = append(images, providers.Image{ID: "aws-ubuntu-" + v, IDsByRegion: map[string]string{d.client.region: x.ID}, Name: x.Name, Family: "ubuntu", Version: v, Architecture: "x86_64", Available: true})
		}
	}
	return providers.Catalog{Regions: []providers.Region{{ID: d.client.region, Name: d.client.region, Available: true}}, Plans: ps, Images: images}, nil
}
