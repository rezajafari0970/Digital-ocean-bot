package aws

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestAWSEmptyCatalogIdentifiesReadStage(t *testing.T) {
	for _, tc := range []struct{ name, action, body, code, operation string }{
		{"types_absent", "DescribeInstanceTypes", "<instanceTypeSet/>", "NO_SUPPORTED_INSTANCE_TYPES", "catalog"},
		{"offerings_absent", "DescribeInstanceTypeOfferings", "<instanceTypeOfferingSet/>", "NO_SUPPORTED_INSTANCE_TYPES", "catalog"},
		{"images_absent", "DescribeImages", "<imagesSet/>", "NO_UBUNTU_IMAGES", "DescribeImages"},
		{"images_rejected", "DescribeImages", strings.Replace(imageXML, "099720109477", "999999999999", 1), "NO_SUPPORTED_UBUNTU_IMAGES", "DescribeImages"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			d := openTest(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" {
					t.Fatal("catalog mutation")
				}
				action := query(r).Get("Action")
				if action == tc.action {
					return response(action, tc.body), nil
				}
				return response(action, fixtureRead(action)), nil
			})
			c, err := d.Catalog(context.Background())
			if err == nil || len(c.Images) != 0 || len(c.Plans) != 0 || calls > 6 {
				t.Fatal("empty catalog accepted or retried", c, err, calls)
			}
			diag := Diagnostic(err)
			if diag.Code != tc.code || diag.Operation != tc.operation || diag.RequiredPermission != "" || !strings.Contains(diag.Guidance, "selected region") {
				t.Fatalf("misleading catalog diagnostic: %+v", diag)
			}
		})
	}
}
