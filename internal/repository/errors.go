package repository

import "errors"

var (
	ErrSeatsUnavailable = errors.New("some seats unavailable")
	ErrHoldExpired      = errors.New("hold expired")
	ErrNoSeatsInHold    = errors.New("no seats in hold")
	ErrNothingToConfirm = errors.New("nothing to confirm")
	ErrNotFound         = errors.New("not found")
	ErrConflict         = errors.New("conflict")
)
