package accounts

import (
	"errors"
	"fmt"
)

var ErrTenantMismatch = errors.New("cross-account access denied")

type Context struct {
	AccountID       string
	SecretNamespace string
	CacheNamespace  string
	JobNamespace    string
	RateNamespace   string
	AuditNamespace  string
}

func NewContext(accountID string) Context {
	return Context{
		AccountID:       accountID,
		SecretNamespace: fmt.Sprintf("account:%s:secrets", accountID),
		CacheNamespace:  fmt.Sprintf("account:%s:cache", accountID),
		JobNamespace:    fmt.Sprintf("account:%s:jobs", accountID),
		RateNamespace:   fmt.Sprintf("account:%s:ratelimit", accountID),
		AuditNamespace:  fmt.Sprintf("account:%s:audit", accountID),
	}
}

func (c Context) Authorize(accountID string) error {
	if c.AccountID == "" || c.AccountID != accountID {
		return ErrTenantMismatch
	}
	return nil
}
