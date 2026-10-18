package transportcore

import "encoding/json"

// CoreHashes returns the release-pinned native binaries accepted as runtime evidence.
func CoreHashes() []string {
	var m struct {
		Core struct{ SHA256 string }
		Arm  struct{ SHA256 string } `json:"core_arm64"`
	}
	if json.Unmarshal(manifest, &m) != nil {
		return nil
	}
	return []string{m.Core.SHA256, m.Arm.SHA256}
}
