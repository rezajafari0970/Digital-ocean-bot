package sanaei

import (
	"context"
	"testing"
)

func TestReadInventoryNormalizesWithoutSecrets(
	t *testing.T,
) {

	body := []byte(
		`{
"success": true,
"obj": [
{
"id": 7,
"remark": "reality-443",
"protocol": "vless",
"port": 443,
"listen": "",
"enable": true,
"up": 10,
"down": 20,
"total": 30,
"settings": "{\"clients\":[{\"id\":\"sensitive-client-id\"}]}",
"streamSettings": "{}",
"clientStats": [
{}
]
}
]
}`,
	)

	snapshot, err := ReadInventory(
		context.Background(),
		fakeExecutor{
			resp: SessionResponse{
				StatusCode: 200,
				Body:       body,
			},
		},
		"panel-1",
	)

	if err != nil {
		t.Fatal(err)
	}

	if len(snapshot.Records) != 1 {
		t.Fatalf(
			"unexpected record count: %d",
			len(snapshot.Records),
		)
	}

	record := snapshot.Records[0]

	if record.RemoteID != 7 {
		t.Fatalf(
			"unexpected remote id: %d",
			record.RemoteID,
		)
	}

	if record.Protocol != "vless" {
		t.Fatalf(
			"unexpected protocol: %s",
			record.Protocol,
		)
	}

	if record.Port != 443 {
		t.Fatalf(
			"unexpected port: %d",
			record.Port,
		)
	}

	if record.ClientCount != 1 {
		t.Fatalf(
			"unexpected client count: %d",
			record.ClientCount,
		)
	}

	if record.RawHash == "" {
		t.Fatal(
			"raw hash missing",
		)
	}
}
