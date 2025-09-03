package reservation

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var (
	ErrSeatsUnavailable = errors.New("some seats are unavailable")
	ErrHoldConflict     = errors.New("conflict creating hold")
	ErrHoldNotFound     = errors.New("hold not found")
	ErrHoldExpired      = errors.New("hold is expired")
	ErrEventNotFound    = errors.New("event not found")
)

type NoSeatsAvailableError struct{}

func (e NoSeatsAvailableError) Error() string {
	return "no seats available"
}

type SeatsUnavailableError struct {
	SeatIDs []int64
}

func (e SeatsUnavailableError) Error() string {
	return fmt.Sprintf("some or all seats are unavailable: %v", e.SeatIDs)
}

type HoldNotFoundError struct {
	HoldID uuid.UUID
}

func (e HoldNotFoundError) Error() string {
	return fmt.Sprintf("hold not found: %s", e.HoldID)
}

type SeatsNotFoundError struct {
	SeatIDs []int64
}

func (e SeatsNotFoundError) Error() string {
	return fmt.Sprintf("seats not found: %v", e.SeatIDs)
}

type EventNotFoundError struct {
	EventID int64
}

func (e EventNotFoundError) Error() string {
	return fmt.Sprintf("event not found: %d", e.EventID)
}

type ConflictError struct{}

func (e ConflictError) Error() string {
	return "conflict"
}
