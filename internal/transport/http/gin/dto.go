package httpgin

import (
	"encoding/json"
	"time"
)

type CreateHoldRequest struct {
	UserID  int64   `json:"user_id" binding:"required"`
	SeatIDs []int64 `json:"seat_ids" binding:"required,min=1,dive,required"`
	TTLSec  int     `json:"ttl_sec"`
}

type ConfirmOrderRequest struct {
	HoldID     string `json:"hold_id" binding:"required,uuid"`
	TotalCents int    `json:"total_cents" binding:"required,gt=0"`
}

type CreateVenueRequest struct {
	Name          string          `json:"name" binding:"required"`
	SeatingScheme json.RawMessage `json:"seating_scheme"`
}

type BatchCreateSeatsRequest struct {
	Seats []SeatInput `json:"seats" binding:"required,min=1,dive"`
}

type SeatInput struct {
	Section string `json:"section" binding:"required"`
	Row     string `json:"row" binding:"required"`
	Number  int    `json:"number" binding:"required,gt=0"`
}

type CreateEventRequest struct {
	VenueID  int64  `json:"venue_id" binding:"required"`
	Title    string `json:"title" binding:"required"`
	StartsAt string `json:"starts_at" binding:"required"`
	EndsAt   string `json:"ends_at" binding:"required"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type CreateHoldResponse struct {
	HoldID string `json:"hold_id"`
}

type ConfirmOrderResponse struct {
	OrderID string `json:"order_id"`
	EventID int64  `json:"event_id"`
}

type CreateVenueResponse struct {
	VenueID int64 `json:"venue_id"`
}

type CreateEventResponse struct {
	EventID int64 `json:"event_id"`
}

func parseRFC3339(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}
