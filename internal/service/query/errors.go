package query

import (
	"errors"
)

var (
	ErrEventNotFound = errors.New("event not found")
	ErrOrderNotFound = errors.New("order not found")
)
