package aws

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

// EC2 permits an EBS root together with optional instance-store mappings.
// Wire names and semantics: AWS API BlockDeviceMapping and User Guide
// block-device-mapping-concepts. This is a synthetic, credential-free response.
const rootMapping = "<item><deviceName>/dev/sda1</deviceName><ebs><volumeSize>8</volumeSize></ebs></item>"
const ephemeralMapping = "<item><deviceName>/dev/sdb</deviceName><virtualName>ephemeral0</virtualName></item>"

func imageWithMappings(mappings string) string {
	return strings.Replace(imageXML, rootMapping, mappings, 1)
}

func TestAWSImageMappingCatalogAndCreate(t *testing.T) {
	for _, tc := range []struct {
		name, mappings string
		usable         bool
	}{
		{"root_only", rootMapping, true},
		{"root_and_instance_store", rootMapping + ephemeralMapping, true},
		{"instance_store_before_root", ephemeralMapping + rootMapping, true},
		{"two_instance_stores", rootMapping + ephemeralMapping + "<item><deviceName>/dev/sdc</deviceName><virtualName>ephemeral1</virtualName></item>", true},
		{"suppressed_optional_device", rootMapping + "<item><deviceName>/dev/sdc</deviceName><noDevice/></item>", true},
		{"extra_ebs", rootMapping + "<item><deviceName>/dev/sdc</deviceName><ebs><volumeSize>8</volumeSize></ebs></item>", false},
		{"missing_root", ephemeralMapping, false},
		{"duplicate_root", rootMapping + rootMapping, false},
		{"duplicate_device", rootMapping + strings.Replace(ephemeralMapping, "/dev/sdb", "/dev/sda1", 1), false},
		{"duplicate_virtual", rootMapping + ephemeralMapping + strings.Replace(ephemeralMapping, "/dev/sdb", "/dev/sdc", 1), false},
		{"root_is_ephemeral", strings.Replace(ephemeralMapping, "/dev/sdb", "/dev/sda1", 1), false},
		{"suppressed_root", strings.Replace(rootMapping, "<ebs>", "<noDevice/><ebs>", 1), false},
		{"mixed_ebs_virtual", strings.Replace(rootMapping, "<ebs>", "<virtualName>ephemeral0</virtualName><ebs>", 1), false},
		{"unknown_mapping", rootMapping + "<item><deviceName>/dev/sdc</deviceName></item>", false},
		{"invalid_virtual", rootMapping + strings.Replace(ephemeralMapping, "ephemeral0", "arbitrary", 1), false},
		{"missing_device_name", rootMapping + strings.Replace(ephemeralMapping, "<deviceName>/dev/sdb</deviceName>", "", 1), false},
		{"empty_root_ebs", strings.Replace(rootMapping, "<volumeSize>8</volumeSize>", "", 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts := 0
			d := openTest(t, func(r *http.Request) (*http.Response, error) {
				q := query(r)
				action := q.Get("Action")
				if r.Method == "POST" {
					posts++
					if action != "RunInstances" || q.Get("BlockDeviceMapping.1.DeviceName") != "/dev/sda1" || q.Get("BlockDeviceMapping.1.Ebs.VolumeSize") != "12" || q.Get("BlockDeviceMapping.1.Ebs.DeleteOnTermination") != "true" {
						t.Fatal("unexpected mutation or root volume policy", action)
					}
					for k := range q {
						if strings.HasPrefix(k, "BlockDeviceMapping.") && !strings.HasPrefix(k, "BlockDeviceMapping.1.") {
							t.Fatal("extra volume request", k)
						}
					}
					return response(action, "<instancesSet><item><instanceId>i-12345678</instanceId></item></instancesSet>"), nil
				}
				if action == "DescribeImages" {
					return response(action, imageWithMappings(tc.mappings)), nil
				}
				return response(action, fixtureRead(action)), nil
			})
			catalog, err := d.Catalog(context.Background())
			if tc.usable {
				if err != nil || len(catalog.Images) != 1 {
					t.Fatalf("valid image omitted: %+v %v", catalog, err)
				}
			} else if len(catalog.Images) != 0 {
				t.Fatal("unsafe image offered")
			}
			if posts != 0 {
				t.Fatal("catalog mutated AWS")
			}
			_, err = d.CreateServer(context.Background(), providers.CreateServerRequest{Name: "server", Identity: "dob-mapping-fixture", RegionID: "eu-west-1", PlanID: "t3.micro", ImageID: "ami-12345678", SSHAuthorizedKeys: []string{publicKey(t)}})
			if tc.usable {
				if err != nil || posts != 1 {
					t.Fatal("valid image rejected at create", err, posts)
				}
			} else if !providers.IsClass(err, providers.ErrorInvalidRequest) || posts != 0 {
				t.Fatal("unsafe image passed create fence", err, posts)
			}
		})
	}
}
