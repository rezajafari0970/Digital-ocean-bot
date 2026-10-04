package configprofiles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type Queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}
type Profile struct {
	Class       string `json:"route_class"`
	Enabled     bool   `json:"enabled"`
	Ports       []int  `json:"ports"`
	Target      int    `json:"target_users_per_inbound"`
	Quota       int64  `json:"user_quota_bytes"`
	Lifetime    int    `json:"user_lifetime_seconds"`
	DeviceLimit int    `json:"device_limit"`
	Rate        int    `json:"users_per_second"`
	Revision    int64  `json:"profile_revision"`
}

func (p Profile) Applies(port int) bool {
	for _, v := range p.Ports {
		if v == port {
			return true
		}
	}
	return false
}
func Read(ctx context.Context, db Queryer, lock bool) ([]Profile, error) {
	query := `SELECT route_class,enabled,ports,target_users_per_inbound,user_quota_bytes,user_lifetime_seconds,device_limit,users_per_second,revision FROM reality_config_profiles ORDER BY route_class`
	if lock {
		query += ` FOR SHARE`
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Profile{}
	for rows.Next() {
		var p Profile
		var raw []byte
		if err = rows.Scan(&p.Class, &p.Enabled, &raw, &p.Target, &p.Quota, &p.Lifetime, &p.DeviceLimit, &p.Rate, &p.Revision); err != nil {
			return nil, err
		}
		if json.Unmarshal(raw, &p.Ports) != nil || len(p.Ports) == 0 {
			return nil, errors.New("invalid profile ports")
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
func Target(profiles []Profile, port int) int {
	n := 0
	for _, p := range profiles {
		if p.Enabled && p.Applies(port) {
			n += p.Target
		}
	}
	return n
}
