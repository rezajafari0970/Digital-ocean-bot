package vultr

type paginationMeta struct {
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

type accountResponse struct {
	Account struct {
		Name           string  `json:"name"`
		Email          string  `json:"email"`
		Balance        float64 `json:"balance"`
		PendingCharges float64 `json:"pending_charges"`
	} `json:"account"`
}
type region struct {
	ID        string   `json:"id"`
	City      string   `json:"city"`
	Country   string   `json:"country"`
	Continent string   `json:"continent"`
	Options   []string `json:"options"`
}
type regionsResponse struct {
	Regions []region       `json:"regions"`
	Meta    paginationMeta `json:"meta"`
}
type plan struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	VCPUCount   int      `json:"vcpu_count"`
	RAM         int      `json:"ram"`
	DiskCount   int      `json:"disk_count"`
	Disk        int      `json:"disk"`
	MonthlyCost float64  `json:"monthly_cost"`
	Locations   []string `json:"locations"`
}
type plansResponse struct {
	Plans []plan         `json:"plans"`
	Meta  paginationMeta `json:"meta"`
}
type osItem struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Arch   string `json:"arch"`
	Family string `json:"family"`
}
type osResponse struct {
	OS   []osItem       `json:"os"`
	Meta paginationMeta `json:"meta"`
}
type instance struct {
	ID           string   `json:"id"`
	OS           string   `json:"os"`
	RAM          int      `json:"ram"`
	Disk         int      `json:"disk"`
	MainIP       string   `json:"main_ip"`
	VCPUCount    int      `json:"vcpu_count"`
	Region       string   `json:"region"`
	Plan         string   `json:"plan"`
	DateCreated  string   `json:"date_created"`
	Status       string   `json:"status"`
	PowerStatus  string   `json:"power_status"`
	ServerStatus string   `json:"server_status"`
	Label        string   `json:"label"`
	Tags         []string `json:"tags"`
}
type instancesResponse struct {
	Instances []instance     `json:"instances"`
	Meta      paginationMeta `json:"meta"`
}
type sshKey struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	SSHKey      string `json:"ssh_key"`
	DateCreated string `json:"date_created"`
}
type sshKeysResponse struct {
	SSHKeys []sshKey       `json:"ssh_keys"`
	Meta    paginationMeta `json:"meta"`
}
