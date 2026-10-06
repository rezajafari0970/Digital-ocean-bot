package serverprotection

import (
	"errors"
	"time"
)

const Version = "1.0.0"
const StateDir = "/var/lib/dob-server-guardian"
const PolicyPath = "/etc/dob-server-guardian/policy.json"
const RuntimeDir = "/run/dob-server-guardian"
const BinaryPath = "/usr/local/libexec/dob-server-guardian"
const UnitName = "dob-server-guardian.service"
const XrayConfig = "/usr/local/x-ui/bin/config.json"
const FreshSeconds = 60

type Policy struct {
	Revision      int64  `json:"revision"`
	Enabled       bool   `json:"enabled"`
	PanelID       string `json:"panel_id"`
	ExcludedPorts []int  `json:"excluded_ports"`
}

func (p Policy) Validate() error {
	if p.Revision < 1 || len(p.PanelID) != 36 || len(p.ExcludedPorts) > 32 {
		return errors.New("invalid protection policy")
	}
	for _, n := range p.ExcludedPorts {
		if n < 1 || n > 65535 {
			return errors.New("invalid excluded port")
		}
	}
	return nil
}

type Metrics struct {
	MemTotalMB         float64 `json:"mem_total_mb"`
	MemAvailableMB     float64 `json:"mem_available_mb"`
	SwapUsedMB         float64 `json:"swap_used_mb"`
	MemoryPSI          float64 `json:"memory_psi_percent"`
	CPUPSI             float64 `json:"cpu_psi_percent"`
	IOPSI              float64 `json:"io_psi_percent"`
	CPUPercent         float64 `json:"cpu_percent"`
	CPUHotCorePercent  float64 `json:"cpu_hot_core_percent"`
	CPUStealPercent    float64 `json:"cpu_steal_percent"`
	FDPercent          float64 `json:"fd_percent"`
	ProcessFDPercent   float64 `json:"process_fd_percent"`
	ProcessFDKnown     bool    `json:"process_fd_known"`
	ConntrackPercent   float64 `json:"conntrack_percent"`
	DiskFreeMB         float64 `json:"disk_free_mb"`
	DiskFreePercent    float64 `json:"disk_free_percent"`
	InodeFreePercent   float64 `json:"inode_free_percent"`
	RXMbps             float64 `json:"rx_mbps"`
	TXMbps             float64 `json:"tx_mbps"`
	PacketsPerSecond   float64 `json:"packets_per_second"`
	DroppedPerSecond   float64 `json:"dropped_per_second"`
	PSISupported       bool    `json:"psi_supported"`
	ConntrackSupported bool    `json:"conntrack_supported"`
}
type Status struct {
	Version          string    `json:"version"`
	Revision         int64     `json:"revision"`
	Enabled          bool      `json:"enabled"`
	State            string    `json:"state"`
	Reason           string    `json:"reason"`
	ObservedAt       time.Time `json:"observed_at"`
	SampleAgeMS      int64     `json:"sample_age_ms"`
	AgentRunning     bool      `json:"agent_running"`
	AdmissionBlocked bool      `json:"admission_blocked"`
	NFTSupported     bool      `json:"nft_supported"`
	Ports            []int     `json:"ports"`
	Metrics          Metrics   `json:"metrics"`
	ActuationMS      float64   `json:"actuation_ms"`
	XUIState         string    `json:"xui_state"`
	Recovery         string    `json:"recovery"`
}
