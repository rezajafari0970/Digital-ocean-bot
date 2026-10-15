package secrets

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
)

// Optional private file: {"1":"<base64 old key>", ...}. Values are never logged.
// Candidate removal is checked against retained encrypted archives at bootstrap.
func LoadRetainedKeys() (map[int][]byte, error) {
	out := map[int][]byte{}
	path := strings.TrimSpace(os.Getenv("MASTER_KEYRING_FILE"))
	if path == "" {
		return out, nil
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, ErrMasterKeyUnavailable
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 65536 {
		return nil, errors.New("master keyring must be a private regular file")
	}
	b, e := io.ReadAll(io.LimitReader(f, 65537))
	if e != nil {
		return nil, ErrMasterKeyUnavailable
	}
	defer func() {
		for i := range b {
			b[i] = 0
		}
	}()
	var raw map[string]string
	if json.Unmarshal(b, &raw) != nil {
		return nil, ErrInvalidMasterKey
	}
	for version, encoded := range raw {
		v, e := strconv.Atoi(version)
		if e != nil || v < 1 {
			return nil, ErrInvalidMasterKey
		}
		if _, ok := out[v]; ok {
			return nil, ErrInvalidMasterKey
		}
		key, e := decodeMasterKey(encoded)
		if e != nil {
			return nil, e
		}
		out[v] = key
	}
	return out, nil
}
