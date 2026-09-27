package realityconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidConfig = errors.New(
	"invalid reality inbound config",
)

type Input struct {
	Remark string

	Port   int
	Listen string

	UUID  string
	Email string
	Flow  string

	Target     string
	TargetPort int
	ServerName string

	PrivateKey string
	ShortID    string

	Fingerprint string
}

type Payload struct {
	Enable bool `json:"enable"`

	Remark string `json:"remark"`

	Listen string `json:"listen"`

	Port int `json:"port"`

	Protocol string `json:"protocol"`

	ExpiryTime int64 `json:"expiryTime"`

	Total int64 `json:"total"`

	Settings map[string]any `json:"settings"`

	StreamSettings map[string]any `json:"streamSettings"`

	Sniffing map[string]any `json:"sniffing"`
}

func Build(
	input Input,
) (Payload, error) {

	if input.Remark == "" ||
		input.Port < 1 ||
		input.Port > 65535 ||
		input.UUID == "" ||
		input.Target == "" ||
		input.ServerName == "" ||
		input.PrivateKey == "" ||
		input.ShortID == "" {

		return Payload{},
			ErrInvalidConfig
	}

	if strings.ContainsAny(
		input.Target+
			input.ServerName,
		"/\\ \t\r\n",
	) {
		return Payload{},
			ErrInvalidConfig
	}

	if input.TargetPort == 0 {
		input.TargetPort = 443
	}
	if input.TargetPort < 1 || input.TargetPort > 65535 {
		return Payload{}, ErrInvalidConfig
	}

	if input.Fingerprint == "" {
		input.Fingerprint = "chrome"
	}

	if input.Flow == "" {
		input.Flow =
			"xtls-rprx-vision"
	}

	settings :=
		map[string]any{
			"clients": []any{
				map[string]any{
					"id": input.UUID,

					"email": input.Email,

					"flow": input.Flow,

					"enable": true,
				},
			},

			"decryption": "none",

			"fallbacks": []any{},
		}

	realitySettings :=
		map[string]any{
			"show": false,

			"dest": fmt.Sprintf(
				"%s:443",
				input.Target,
			),

			"xver": 0,

			"serverNames": []string{
				input.ServerName,
			},

			"privateKey": input.PrivateKey,

			"shortIds": []string{
				input.ShortID,
			},

			"fingerprint": input.Fingerprint,
		}

	streamSettings :=
		map[string]any{
			"network": "tcp",

			"security": "reality",

			"realitySettings": realitySettings,
		}

	sniffing :=
		map[string]any{
			"enabled": true,

			"destOverride": []string{
				"http",
				"tls",
				"quic",
			},

			"metadataOnly": false,

			"routeOnly": false,
		}

	return Payload{
		Enable: true,

		Remark: input.Remark,

		Listen: input.Listen,

		Port: input.Port,

		Protocol: "vless",

		ExpiryTime: 0,

		Total: 0,

		Settings: settings,

		StreamSettings: streamSettings,

		Sniffing: sniffing,
	}, nil
}

func Marshal(
	input Input,
) ([]byte, error) {

	payload, err :=
		Build(
			input,
		)

	if err != nil {
		return nil, err
	}

	return json.Marshal(
		payload,
	)
}
