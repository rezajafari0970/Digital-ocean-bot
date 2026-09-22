package resources

import "context"

type Registry interface {
	Sync(ctx context.Context, resources []Resource) error
	Find(ctx context.Context, accountID string) ([]Resource, error)
}

type Reconciler struct{}

func (Reconciler) Compare(local, remote []Resource) []Difference {
	return []Difference{}
}
