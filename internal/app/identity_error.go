package app

import "errors"

type sqlStateError interface {
	SQLState() string
}

func isIdentityUniqueViolation(err error) bool {
	var x sqlStateError
	return errors.As(err, &x) && x.SQLState() == "23505"
}

func classifyIdentityWriteError(err error) error {
	if isIdentityUniqueViolation(err) {
		return ErrIsolationWait
	}
	return err
}
