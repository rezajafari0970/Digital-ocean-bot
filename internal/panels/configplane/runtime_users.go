package configplane

import (
	_ "embed"
	"encoding/json"
	"errors"
)

//go:embed runtime_users.py
var runtimeUsersScript string

type nativeVLESSUser struct {
	Email string `json:"email"`
	ID    string `json:"id"`
	Flow  string `json:"flow"`
}

func expectedNativeVLESSUsers(payload map[string]any) ([]nativeVLESSUser, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Protocol string
		Settings struct{ Clients []nativeVLESSUser }
	}
	if err = json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	if parsed.Protocol != "vless" {
		return nil, nil
	}
	users := []nativeVLESSUser{}
	emails, ids := map[string]bool{}, map[string]bool{}
	for _, user := range parsed.Settings.Clients {
		if user.ID == "" || user.Email == "" || emails[user.Email] || ids[user.ID] {
			return nil, errors.New("profile_runtime_vless_identity_invalid")
		}
		emails[user.Email] = true
		ids[user.ID] = true
		users = append(users, user)
	}
	return users, nil
}
