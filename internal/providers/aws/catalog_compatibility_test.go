package aws

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"strings"
	"testing"
)

func TestAWSPlanCompatibilityAndAvailabilityZone(t *testing.T) {
	for _, mode := range []string{"compatible", "old_generation", "paravirtual", "instance_store", "different_az", "no_boot_mode", "uefi_only", "ena_required"} {
		t.Run(mode, func(t *testing.T) {
			posts := 0
			d := openTest(t, func(r *http.Request) (*http.Response, error) {
				q := query(r)
				a := q.Get("Action")
				body := fixtureRead(a)
				if a == "DescribeInstanceTypeOfferings" {
					if q.Get("LocationType") != "availability-zone" || q.Get("Filter.1.Value.1") != "eu-west-1a" {
						t.Fatal("offering not tied to selected subnet zone")
					}
					if mode == "different_az" {
						body = strings.ReplaceAll(body, "eu-west-1a", "eu-west-1b")
					}
				}
				if a == "DescribeInstanceTypes" {
					switch mode {
					case "old_generation":
						body = strings.Replace(body, "<currentGeneration>true", "<currentGeneration>false", 1)
					case "paravirtual":
						body = strings.Replace(body, ">hvm<", ">paravirtual<", 1)
					case "instance_store":
						body = strings.Replace(body, ">ebs<", ">instance-store<", 1)
					case "no_boot_mode":
						body = strings.Replace(body, "<supportedBootModes><item>legacy-bios</item><item>uefi</item></supportedBootModes>", "", 1)
					case "uefi_only":
						body = strings.Replace(body, "<item>legacy-bios</item>", "", 1)
					case "ena_required":
						body = strings.Replace(body, "</instanceType>", "</instanceType><networkInfo><enaSupport>required</enaSupport></networkInfo>", 1)
					}
				}
				if a == "RunInstances" {
					posts++
					return response(a, `<instancesSet><item><instanceId>i-12345678</instanceId></item></instancesSet>`), nil
				}
				return response(a, body), nil
			})
			cat, e := d.Catalog(context.Background())
			if mode == "compatible" {
				if e != nil || len(cat.Plans) != 1 || cat.Plans[0].DiskGB != 0 {
					t.Fatal(cat, e)
				}
			} else if e == nil {
				t.Fatal("incompatible plan offered")
			}
			result, e := d.CreateServer(context.Background(), providers.CreateServerRequest{Name: "fixture", Identity: "fixture-deployment", RegionID: "eu-west-1", PlanID: "t3.micro", ImageID: "ami-12345678", SSHAuthorizedKeys: []string{publicKey(t)}})
			if mode == "compatible" {
				if e != nil || posts != 1 {
					t.Fatal(e, posts)
				}
			} else if e == nil || posts != 0 || !result.ComputeNotDispatched {
				t.Fatal("incompatible plan mutated", e, posts, result)
			}
		})
	}
}
