package buildinfo

import "runtime/debug"

var (
	Commit    = "unknown"
	BuildTime = "unknown"
)

type Info struct {
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version,omitempty"`
	Modified  bool   `json:"modified"`
}

func Current() Info {
	i := Info{Commit: Commit, BuildTime: BuildTime}
	if bi, ok := debug.ReadBuildInfo(); ok {
		i.GoVersion = bi.GoVersion
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if i.Commit == "unknown" && s.Value != "" {
					i.Commit = s.Value
				}
			case "vcs.modified":
				i.Modified = s.Value == "true"
			}
		}
	}
	return i
}
