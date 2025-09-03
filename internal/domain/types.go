package domain

import (
	"time"

	"github.com/google/uuid"
)

type SeatStatus string

const (
	SeatAvailable SeatStatus = "available"
	SeatHeld      SeatStatus = "held"
	SeatSold      SeatStatus = "sold"
)

type Venue struct {
	ID            int64
	Name          string
	SeatingScheme []byte // jsonb raw
}

type Event struct {
	ID      int64
	VenueID int64
	Title   string
	Starts  time.Time
	Ends    time.Time
}

type Seat struct {
	ID      int64
	VenueID int64
	Section string
	Row     string
	Number  int
}

type SeatWithStatus struct {
	Seat
	Status SeatStatus
}

type EventCounts struct {
	Available int64
	Held      int64
	Sold      int64
	Total     int64
}

type Order struct {
	ID         uuid.UUID
	EventID    int64
	UserID     int64
	TotalCents int
	CreatedAt  time.Time
}

type Ticket struct {
	ID      uuid.UUID
	OrderID uuid.UUID
	EventID int64
	SeatID  int64
	Created time.Time
}

type OrderWithTickets struct {
	Order   Order
	Tickets []Ticket
}
