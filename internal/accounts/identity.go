package accounts

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
)

var ErrIdentifierGeneration = errors.New("secure identifier generation failed")

func SecureID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", ErrIdentifierGeneration
	}
	return hex.EncodeToString(b), nil
}

type RuntimeIdentity struct {
	CellID           string
	RequestScope     string
	CorrelationScope string
	AuditScope       string
}

func NewRuntimeIdentity() (RuntimeIdentity, error) {
	cell, err := SecureID()
	if err != nil {
		return RuntimeIdentity{}, err
	}
	req, err := SecureID()
	if err != nil {
		return RuntimeIdentity{}, err
	}
	corr, err := SecureID()
	if err != nil {
		return RuntimeIdentity{}, err
	}
	audit, err := SecureID()
	if err != nil {
		return RuntimeIdentity{}, err
	}
	return RuntimeIdentity{CellID: cell, RequestScope: req, CorrelationScope: corr, AuditScope: audit}, nil
}
