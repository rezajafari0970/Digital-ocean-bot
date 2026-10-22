package providers

import "context"

// Authorization is separate from read-only discovery. Starting it is an
// explicitly journaled credential mutation; checking it is read-only.
type AuthorizationGrant struct {
	Credentials   CredentialSource `json:"-"`
	ValidationURL string           `json:"-"`
}
type AuthorizationFactory interface {
	StartAuthorization(context.Context, OpenRequest) (AuthorizationGrant, error)
	CheckAuthorization(context.Context, OpenRequest) (bool, error)
}

func (r *Registry) Authorizer(name string) (AuthorizationFactory, bool) {
	r.mu.RLock()
	f := r.factories[normalizeName(name)]
	r.mu.RUnlock()
	a, ok := f.(AuthorizationFactory)
	return a, ok
}
