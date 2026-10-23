package ovh

import "regexp"

// Capability policy is independent of catalog discovery. Unknown families are
// visible, but cannot accidentally inherit an unverified provisioning contract.
var localX86Flavor = regexp.MustCompile(`^([bcr][23]|d2|s1|i1)-[0-9]+(-flex)?$`)

func flavorSupport(f flavorData) string {
	if f.OS != "linux" {
		return "Requires a non-Ubuntu operating-system driver"
	}
	if !localX86Flavor.MatchString(f.Name) {
		return "Requires a dedicated GPU, bare-metal or architecture contract"
	}
	if f.Disk <= 0 {
		return "Requires a volume-backed boot and cleanup contract"
	}
	if f.CPU <= 0 || f.RAM <= 0 || !uuidPattern.MatchString(f.ID) || !regionPattern.MatchString(f.Region) {
		return "Invalid provider flavor metadata"
	}
	return ""
}
func usableFlavor(f flavorData) bool { return flavorSupport(f) == "" && f.Available }
