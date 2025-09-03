package orders

import (
	"context"
	"errors"
	"fmt"

	"github.com/kirinyoku/tix-go/internal/domain"
	"github.com/kirinyoku/tix-go/internal/repository"
	postgresrepo "github.com/kirinyoku/tix-go/internal/repository/postgres"
)

type Service struct {
	store *postgresrepo.Store
}

func New(store *postgresrepo.Store) *Service {
	return &Service{store: store}
}

// GetOrderWithTickets retrieves an order along with its associated tickets.
//
// Parameters:
//   - ctx: request-scoped context.
//   - orderID: ID of the order to retrieve.
//
// Returns:
//   - *domain.OrderWithTickets: the retrieved order with tickets, or nil if not found.
//   - error: orders.ErrOrderNotFound if the order is not found.
func (s *Service) GetOrderWithTickets(ctx context.Context, orderID string) (*domain.OrderWithTickets, error) {
	const op = "service.orders.GetOrderWithTickets"

	o, err := s.store.Query().GetOrderWithTickets(ctx, orderID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%s: %w", op, ErrOrderNotFound)
		}

		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return o, nil
}
