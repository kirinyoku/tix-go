package repository

import "errors"

var (
	ErrNotFound         = errors.New("not found")
	ErrConflict         = errors.New("conflict")
	ErrSeatsUnavailable = errors.New("some seats unavailable")
	ErrHoldExpired      = errors.New("hold expired")
	ErrHoldNotFound     = errors.New("hold not found")
	ErrNoSeatsInHold    = errors.New("no seats in hold")
	ErrNothingToConfirm = errors.New("nothing to confirm")
)
