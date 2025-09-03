package admin

import (
	"errors"
)

var (
	ErrVenueConflict          = errors.New("venue already exists")
	ErrSeatsConflict          = errors.New("some seats already exist")
	ErrEventConflict          = errors.New("event already exists")
	ErrFailedToInitEventSeats = errors.New("event or venue does not exist")
)
