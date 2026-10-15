package export

import (
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
	"net"
	"net/url"
	"strconv"
	"strings"
)

var ErrInvalidVLESS = errors.New("invalid vless export")

type VLESSReality struct {
	Transport   transportprofile.Config
	UUID        string
	Host        string
	Port        int
	SNI         string
	PublicKey   string
	ShortID     string
	Fingerprint string
	Encryption  string
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
	// URI is a client-side setting. The server keeps ordinary Vision; this
	// suffix permits QUIC instead of silently rejecting UDP port 443.
	if err := x.Transport.Validate(x.Transport.NetworkName() == "xhttp"); err != nil {
		return "", err
	}
	if x.Transport.NetworkName() == "xhttp" {
		if x.Flow != x.Transport.Flow {
			return "", ErrInvalidVLESS
		}
		if x.Flow != "" {
			if x.Flow != transportprofile.Vision && x.Flow != transportprofile.VisionUDP443 {
				return "", ErrInvalidVLESS
			}
			if !transportprofile.ValidClientEncryption(x.Encryption) {
				return "", ErrInvalidVLESS
			}
		}
	}
	if x.Transport.NetworkName() == "tcp" && (x.Flow == "" || x.Flow == "xtls-rprx-vision") {
		x.Flow = "xtls-rprx-vision-udp443"
	}
	q := url.Values{}
	if x.Encryption == "" {
		x.Encryption = "none"
	}
	if x.Encryption != "none" && !transportprofile.ValidClientEncryption(x.Encryption) {
		return "", ErrInvalidVLESS
	}
	q.Set("encryption", x.Encryption)
	if x.Flow != "" {
		q.Set("flow", x.Flow)
	}
	q.Set("security", "reality")
	q.Set("sni", x.SNI)
	q.Set("fp", x.Fingerprint)
	q.Set("pbk", x.PublicKey)
	q.Set("sid", x.ShortID)
	q.Set("type", x.Transport.NetworkName())
	if x.Transport.NetworkName() == "xhttp" {
		q.Set("mode", x.Transport.Mode)
		q.Set("host", x.Transport.Host)
		q.Set("path", x.Transport.Path)
		q.Set("extra", string(x.Transport.Extra))
		if x.Transport.MaskOnClient() {
			mask, err := x.Transport.TCPMask()
			if err != nil {
				return "", err
			}
			q.Set("fm", string(mask))
		}
	}
	hostport := net.JoinHostPort(x.Host, strconv.Itoa(x.Port))
	return fmt.Sprintf("vless://%s@%s?%s#%s", url.PathEscape(x.UUID), hostport, q.Encode(), url.QueryEscape(x.Remark)), nil
}
