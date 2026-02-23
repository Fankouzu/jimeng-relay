package repository

import "errors"

var ErrNotFound = errors.New("repository: not found")

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
