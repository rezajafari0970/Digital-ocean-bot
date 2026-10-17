package providers

import "testing"

func TestRegionCountryProviderMetadataAndLegacy(t *testing.T) {
	for _, tc := range []struct{ provider, id, name, explicit, want string }{
		{"digitalocean", "fra1", "", "", "DE"}, {"digitalocean", "sfo1", "", "", "US"}, {"digitalocean", "syd1", "", "", "AU"},
		{"digitalocean", "fra99", "", "", ""}, {"digitalocean", "unknown", "Frankfurt", "", ""},
		{"linode", "us-lax", "Los Angeles, CA", "", "US"}, {"linode", "us-southeast", "Atlanta, GA", "", "US"},
		{"linode", "us-ord", "Chicago, IL", "", "US"}, {"linode", "us-east", "Newark, NJ", "", "US"},
		{"linode", "ap-south", "Singapore, SG", "", "SG"}, {"linode", "ap-southeast", "", "", "AU"},
		{"linode", "ap-northeast", "", "", "JP"}, {"linode", "ap-west", "", "", "IN"}, {"linode", "eu-central", "", "", "DE"},
		{"linode", "eu-west", "", "", "GB"}, {"linode", "ca-central", "", "", "CA"}, {"linode", "new-zone", "", "br", "BR"},
		{"vultr", "fra", "Frankfurt, DE", "", "DE"}, {"vultr", "sao", "São Paulo, BR", "", "BR"},
		{"vultr", "future", "", "nz", "NZ"}, {"vultr", "ca", "Canada", "", ""}, {"vultr", "fra", "Frankfurt, DE", "ZZ", ""},
		{"upcloud", "uk-lon1", "", "", "GB"}, {"upcloud", "fi-hel1", "", "", "FI"}, {"upcloud", "de-fra1", "", "", "DE"},
		{"upcloud", "zz-future", "", "", ""}, {"other", "us-east", "City, US", "", ""},
	} {
		t.Run(tc.provider+"/"+tc.id, func(t *testing.T) {
			if got := RegionCountry(tc.provider, tc.id, tc.name, tc.explicit); got != tc.want {
				t.Fatal(got, tc.want)
			}
		})
	}
}
