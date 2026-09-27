package export

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

var ErrInvalidVLESS = errors.New("invalid vless export")

type VLESSReality struct {
	UUID        string
	Host        string
	Port        int
	SNI         string
	PublicKey   string
	ShortID     string
	Fingerprint string
	Flow        string
	Remark      string
}

func VLESSRealityURI(x VLESSReality) (string, error) {
	if x.UUID == "" || x.Host == "" || x.Port < 1 || x.Port > 65535 || x.SNI == "" || x.PublicKey == "" || x.ShortID == "" {
		return "", ErrInvalidVLESS
	}
	if strings.ContainsAny(x.Host+x.SNI, "/\\ \t\r\n") {
		return "", ErrInvalidVLESS
	}
	if x.Fingerprint == "" {
		x.Fingerprint = "chrome"
	}
	if x.Flow == "" {
		x.Flow = "xtls-rprx-vision"
	}
	q := url.Values{}
	q.Set("encryption", "none")
	q.Set("flow", x.Flow)
	q.Set("security", "reality")
	q.Set("sni", x.SNI)
	q.Set("fp", x.Fingerprint)
	q.Set("pbk", x.PublicKey)
	q.Set("sid", x.ShortID)
	q.Set("type", "tcp")
	hostport := net.JoinHostPort(x.Host, strconv.Itoa(x.Port))
	return fmt.Sprintf("vless://%s@%s?%s#%s", url.PathEscape(x.UUID), hostport, q.Encode(), url.QueryEscape(x.Remark)), nil
}
