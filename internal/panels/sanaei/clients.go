package sanaei

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

var ErrClientCount = errors.New("invalid client count")

type Client struct {
	ID         string `json:"id"`
	Email      string `json:"email"`
	Enable     bool   `json:"enable"`
	TotalGB    int64  `json:"totalGB"`
	ExpiryTime int64  `json:"expiryTime"`
	LimitIP    int    `json:"limitIp"`
	Flow       string `json:"flow,omitempty"`
}
type ClientRecord struct {
	ID        string
	AccountID string
	DropletID string
	InboundID int
	Email     string
	Enabled   bool
	CreatedAt time.Time
}

type ClientStore interface {
	Save(context.Context, ClientRecord) error
}

type ClientManager struct {
	API   *APIClient
	Store ClientStore
}

func (m ClientManager) CreateMany(ctx context.Context, accountID, dropletID string, inboundID, count int, emailPrefix string) ([]ClientRecord, error) {
	if count < 1 || count > 10000 {
		return nil, ErrClientCount
	}
	out := make([]ClientRecord, 0, count)
	for i := 0; i < count; i++ {
		id, err := UUIDv4()
		if err != nil {
			return out, err
		}
		email := emailPrefix + "-" + id[:8]
		client := Client{ID: id, Email: email, Enable: true}
		if err := m.API.AddClient(ctx, inboundID, client); err != nil {
			return out, err
		}
		record := ClientRecord{ID: id, AccountID: accountID, DropletID: dropletID, InboundID: inboundID, Email: email, Enabled: true, CreatedAt: time.Now().UTC()}
		if err := m.Store.Save(ctx, record); err != nil {
			return out, err
		}
		out = append(out, record)
	}
	return out, nil
}

func UUIDv4() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b)
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32], nil
}
