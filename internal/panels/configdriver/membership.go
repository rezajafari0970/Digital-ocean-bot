package configdriver

import "errors"

// Resize retains the first existing identities in each class, in persisted
// order. Added identities are generated once and persisted before native use.
func Resize(previous Credentials, profile, panel string, classes []string, count int) (Credentials, error) {
	if count < 1 || count > 100 || len(classes) < 1 || len(classes) > 2 {
		return Credentials{}, errors.New("profile_client_count_invalid")
	}
	out := previous
	out.Clients = nil
	seen := map[string]bool{}
	for _, class := range classes {
		if seen[class] || (class != "DIRECT" && class != "RESIDENTIAL") {
			return Credentials{}, errors.New("profile_class_invalid")
		}
		seen[class] = true
		members := []Client{}
		for _, c := range previous.Clients {
			if c.Class == class && len(members) < count {
				members = append(members, c)
			}
		}
		if len(members) < count {
			added, e := NewCredentials(profile, panel, []string{class}, count-len(members))
			if e != nil {
				return Credentials{}, e
			}
			members = append(members, added.Clients...)
		}
		out.Clients = append(out.Clients, members...)
	}
	return out, nil
}
