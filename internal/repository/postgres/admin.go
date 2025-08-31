package postgresrepo

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kirinyoku/tix-go/internal/domain"
)

type AdminRepo struct {
	pool *pgxpool.Pool
	db   DB
}

func (r *AdminRepo) With(db DB) *AdminRepo {
	cp := *r
	cp.db = db
	return &cp
}

func (r *AdminRepo) handle() DB {
	if r.db != nil {
		return r.db
	}
	return r.pool
}

func (r *AdminRepo) CreateVenue(
	ctx context.Context,
	name string,
	seatingSchemeJSON []byte,
) (int64, error) {
	const op = "postgresrepo.AdminRepo.CreateVenue"

	db := r.handle()

	var id int64
	if err := db.QueryRow(ctx,
		`INSERT INTO venues(name, seating_scheme)
       	 VALUES ($1, $2)
     	 RETURNING id`,
		name, seatingSchemeJSON,
	).Scan(&id); err != nil {
		return 0, wrapDBErr(op, err)
	}

	return id, nil
}

func (r *AdminRepo) BatchCreateSeats(
	ctx context.Context,
	venueID int64,
	seats []domain.Seat,
) error {
	const op = "postgresrepo.AdminRepo.BacthCreateSeats"

	db := r.handle()

	batch := &pgx.Batch{}
	for _, s := range seats {
		batch.Queue(
			`INSERT INTO seats(venue_id, section, row, number)
         	 VALUES ($1, $2, $3, $4)
       		 ON CONFLICT (venue_id, section, row, number) DO NOTHING`,
			venueID, s.Section, s.Row, s.Number,
		)
	}
	if err := db.SendBatch(ctx, batch).Close(); err != nil {
		return wrapDBErr(op, err)
	}

	return nil
}

func (r *AdminRepo) CreateEvent(
	ctx context.Context,
	venueID int64,
	title string,
	starts, ends any,
) (int64, error) {
	const op = "postgresrepo.AdminRepo.CreateEvent"

	db := r.handle()

	var id int64
	if err := db.QueryRow(ctx,
		`INSERT INTO events(venue_id, title, starts_at, ends_at)
       	 VALUES ($1, $2, $3, $4)
     	 RETURNING id`,
		venueID, title, starts, ends,
	).Scan(&id); err != nil {
		return 0, wrapDBErr(op, err)
	}

	return id, nil
}

func (r *AdminRepo) InitEventSeats(
	ctx context.Context,
	eventID int64,
	venueID int64,
) (int64, error) {
	const op = "postgresrepo.AdminRepo.InitEventSeats"

	db := r.handle()

	tag, err := db.Exec(ctx,
		`INSERT INTO event_seats(event_id, seat_id, status)
       	 SELECT $1, s.id, 'available'
         FROM seats s
         WHERE s.venue_id = $2
     	 ON CONFLICT DO NOTHING`,
		eventID, venueID,
	)
	if err != nil {
		return 0, wrapDBErr(op, err)
	}

	return tag.RowsAffected(), nil
}
