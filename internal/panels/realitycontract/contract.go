package realitycontract

import (
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"

	exporter "github.com/rezajafari0970/Digital-ocean-bot/internal/panels/export"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/realityconfig"
)

var ErrInvalid = errors.New("invalid managed reality contract")

type Managed struct {
	Transport   transportprofile.Config
	Remark      string
	Host        string
	Port        int
	UUID        string
	Email       string
	Target      string
	TargetPort  int
	SNI         string
	ServerNames []string
	PrivateKey  string
	PublicKey   string
	ShortID     string
	Encryption  string
	Flow        string
	Fingerprint string
}

func (m Managed) normalized() Managed {
	if m.Flow == "" && m.Transport.NetworkName() == "tcp" {
		m.Flow = "xtls-rprx-vision"
	}
	if m.Fingerprint == "" {
		m.Fingerprint = "chrome"
	}
	if m.TargetPort == 0 {
		m.TargetPort = 443
	}
	if len(m.ServerNames) == 0 && m.SNI != "" {
		m.ServerNames = []string{m.SNI}
	}
	return m
}

func (m Managed) Payload() (realityconfig.Payload, error) {
	m = m.normalized()
	if m.PrivateKey == "" {
		return realityconfig.Payload{}, ErrInvalid
	}
	return realityconfig.Build(realityconfig.Input{
		Remark: m.Remark, Port: m.Port, UUID: m.UUID, Email: m.Email, Flow: m.Flow, Transport: m.Transport,
		Target: m.Target, TargetPort: m.TargetPort, ServerName: m.SNI, ServerNames: m.ServerNames,
		PrivateKey: m.PrivateKey, ShortID: m.ShortID, Fingerprint: m.Fingerprint,
	})
}

func (m Managed) URI() (string, error) {
	m = m.normalized()
	if m.PublicKey == "" {
		return "", ErrInvalid
	}
	if m.Transport.NetworkName() == "xhttp" && m.Transport.Flow != "" {
		if m.Flow != "" && m.Flow != m.Transport.ServerFlow() && m.Flow != m.Transport.Flow {
			return "", ErrInvalid
		}
		m.Flow = m.Transport.Flow
		if m.Encryption == "" {
			_, public, err := transportprofile.EncryptionPair(m.PrivateKey, m.Remark)
			if err != nil {
				return "", err
			}
			m.Encryption = public
		}
	}
	return exporter.VLESSRealityURI(exporter.VLESSReality{
		UUID: m.UUID, Host: m.Host, Port: m.Port, SNI: m.SNI, PublicKey: m.PublicKey,
		ShortID: m.ShortID, Fingerprint: m.Fingerprint, Flow: m.Flow, Encryption: m.Encryption, Remark: m.Remark, Transport: m.Transport,
	})
}
