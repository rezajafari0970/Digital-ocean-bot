package aws

import (
	"context"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

const canonicalOwner = "099720109477"

func (d *Driver) CatalogCacheVersion() string { return "aws-regional-catalog-v2" }

type image struct {
	BootMode       string `xml:"bootMode"`
	ENA            bool   `xml:"enaSupport"`
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
		Device      string  `xml:"deviceName"`
		VirtualName string  `xml:"virtualName"`
		NoDevice    *string `xml:"noDevice"`
		EBS         *struct {
			Size int `xml:"volumeSize"`
		} `xml:"ebs"`
	} `xml:"blockDeviceMapping>item"`
}

func (x image) RootSize() int {
	for _, b := range x.Devices {
		if b.Device == x.Root && b.EBS != nil {
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

// AMIs may include instance-store mappings even when the selected instance
// has no instance store. Only EBS mappings allocate persistent volumes; retain
// the single-root-EBS contract without rejecting optional ephemeral devices.
var ephemeralDevice = regexp.MustCompile(`^ephemeral([0-9]|1[0-9]|2[0-3])$`)

func (x image) singleRootEBS() bool {
	root := false
	devices, virtual := map[string]bool{}, map[string]bool{}
	for _, b := range x.Devices {
		if b.Device == "" || devices[b.Device] {
			return false
		}
		devices[b.Device] = true
		if b.Device == x.Root {
			if b.EBS == nil || b.EBS.Size <= 0 || b.NoDevice != nil || b.VirtualName != "" {
				return false
			}
			root = true
			continue
		}
		if b.EBS != nil {
			return false
		}
		if b.NoDevice != nil {
			if b.VirtualName != "" {
				return false
			}
			continue
		}
		if !ephemeralDevice.MatchString(b.VirtualName) || virtual[b.VirtualName] {
			return false
		}
		virtual[b.VirtualName] = true
	}
	return root
}

func usableImage(x image) bool {
	return idPattern.MatchString(x.ID) && strings.HasPrefix(x.ID, "ami-") && x.Owner == canonicalOwner && x.Public && x.State == "available" && x.Architecture == "x86_64" && x.RootType == "ebs" && x.Virtualization == "hvm" && x.Root != "" && x.singleRootEBS() && len(x.Products) == 0 && imageVersion(x.Name) != ""
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
	Current        bool     `xml:"currentGeneration"`
	Roots          []string `xml:"supportedRootDeviceTypes>item"`
	Virtualization []string `xml:"supportedVirtualizationTypes>item"`
	BootModes      []string `xml:"supportedBootModes>item"`
	Network        struct {
		ENA string `xml:"enaSupport"`
	} `xml:"networkInfo"`
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

func (d *Driver) plans(ctx context.Context, selected string, images ...image) ([]providers.Plan, error) {
	q := url.Values{"Filter.1.Name": {"processor-info.supported-architecture"}, "Filter.1.Value.1": {"x86_64"}, "Filter.2.Name": {"supported-usage-class"}, "Filter.2.Value.1": {"on-demand"}}
	q.Set("Filter.4.Name", "supported-root-device-type")
	q.Set("Filter.4.Value.1", "ebs")
	q.Set("Filter.5.Name", "supported-virtualization-type")
	q.Set("Filter.5.Value.1", "hvm")
	q.Set("Filter.6.Name", "current-generation")
	q.Set("Filter.6.Value.1", "true")
	if selected != "" {
		q.Set("Filter.3.Name", "instance-type")
		q.Set("Filter.3.Value.1", selected)
	}
	xs, e := pages(ctx, d.client, "DescribeInstanceTypes", "instanceTypeSet", q, func(x instanceType) string { return x.ID })
	if e != nil {
		return nil, e
	}
	zone, e := d.subnetZone(ctx)
	if e != nil {
		return nil, e
	}
	offers, e := pages(ctx, d.client, "DescribeInstanceTypeOfferings", "instanceTypeOfferingSet", url.Values{"LocationType": {"availability-zone"}, "Filter.1.Name": {"location"}, "Filter.1.Value.1": {zone}}, func(x struct {
		ID       string `xml:"instanceType"`
		Location string `xml:"location"`
	}) string {
		return x.ID
	})
	if e != nil {
		return nil, e
	}
	available := map[string]bool{}
	for _, o := range offers {
		if o.Location != zone {
			return nil, problem(providers.ErrorUnavailable, "catalog", "REGION_MISMATCH", 200)
		}
		available[o.ID] = true
	}
	out := []providers.Plan{}
	for _, x := range xs {
		compatible := true
		for _, im := range images {
			if !x.supports(im) {
				compatible = false
			}
		}
		if compatible && x.Current && contains(x.Roots, "ebs") && contains(x.Virtualization, "hvm") && available[x.ID] && x.CPU.Count > 0 && x.Memory.MB > 0 && contains(x.Processor.Architectures, "x86_64") && contains(x.Usage, "on-demand") {
			out = append(out, providers.Plan{ID: x.ID, Name: x.ID, CPU: x.CPU.Count, MemoryMB: x.Memory.MB, Available: true, AvailableRegions: []string{d.client.region}})
		}
	}
	sortPlans(out)
	return out, nil
}
func sortPlans(out []providers.Plan) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].MemoryMB != out[j].MemoryMB {
			return out[i].MemoryMB < out[j].MemoryMB
		}
		return out[i].ID < out[j].ID
	})
}
func (x instanceType) supports(im image) bool {
	if x.Network.ENA == "required" && !im.ENA {
		return false
	}
	switch im.BootMode {
	case "", "legacy-bios":
		return contains(x.BootModes, "legacy-bios")
	case "uefi":
		return contains(x.BootModes, "uefi")
	case "uefi-preferred":
		return contains(x.BootModes, "uefi") || contains(x.BootModes, "legacy-bios")
	}
	return false
}
func (d *Driver) subnetZone(ctx context.Context) (string, error) {
	cred, e := credentials(ctx, d.client.credentials)
	if e != nil {
		return "", e
	}
	var response struct {
		Items []subnetData `xml:"subnetSet>item"`
	}
	if e = d.client.call(ctx, "ec2", "DescribeSubnets", url.Values{"SubnetId.1": {cred.Subnet}}, false, &response); e != nil {
		return "", e
	}
	if len(response.Items) != 1 || response.Items[0].ID != cred.Subnet || response.Items[0].State != "available" || !strings.HasPrefix(response.Items[0].AZ, d.client.region) || response.Items[0].AZ == d.client.region {
		return "", problem(providers.ErrorInvalidRequest, "catalog", "SUBNET_ZONE_UNAVAILABLE", 0)
	}
	return response.Items[0].AZ, nil
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
	xs, e := d.images(ctx, nil)
	if e != nil {
		return providers.Catalog{}, e
	}
	if len(xs) == 0 {
		return providers.Catalog{}, problem(providers.ErrorImageUnavailable, "DescribeImages", "NO_UBUNTU_IMAGES", 200)
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
	if len(latest) == 0 {
		return providers.Catalog{}, problem(providers.ErrorImageUnavailable, "DescribeImages", "NO_SUPPORTED_UBUNTU_IMAGES", 200)
	}
	images := []providers.Image{}
	compatibleImages := []image{}
	for _, v := range []string{"26.04", "24.04", "22.04"} {
		if x, ok := latest[v]; ok {
			compatibleImages = append(compatibleImages, x)
			images = append(images, providers.Image{ID: "aws-ubuntu-" + v, IDsByRegion: map[string]string{d.client.region: x.ID}, Name: x.Name, Family: "ubuntu", Version: v, Architecture: "x86_64", Available: true})
		}
	}
	ps, e := d.plans(ctx, "", compatibleImages...)
	if e != nil {
		return providers.Catalog{}, e
	}
	if len(ps) == 0 {
		return providers.Catalog{}, problem(providers.ErrorRegionCapacity, "catalog", "NO_SUPPORTED_INSTANCE_TYPES", 0)
	}
	pricing := d.loadPrices(ctx, ps)
	return providers.Catalog{Pricing: &pricing, Regions: []providers.Region{{ID: d.client.region, Name: d.client.region, Available: true}}, Plans: ps, Images: images}, nil
}
