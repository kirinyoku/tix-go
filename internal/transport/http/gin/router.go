package httpgin

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kirinyoku/tix-go/internal/domain"
	redisrepo "github.com/kirinyoku/tix-go/internal/repository/redis"
	"github.com/kirinyoku/tix-go/internal/service"
	"github.com/kirinyoku/tix-go/internal/service/admin"
	"github.com/kirinyoku/tix-go/internal/service/orders"
	"github.com/kirinyoku/tix-go/internal/service/query"
	"github.com/kirinyoku/tix-go/internal/service/reservation"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func NewRouter(
	svcs *service.Services,
	idem *redisrepo.IdempotencyStore,
	logger *slog.Logger,
	middlewares ...gin.HandlerFunc,
) *gin.Engine {
	r := gin.New()

	r.Use(gin.Recovery(), LoggingMiddleware(logger), RequestIDMiddleware(), CORS())
	for _, m := range middlewares {
		if m != nil {
			r.Use(m)
		}
	}

	// Swagger UI
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// health
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Public API
	r.GET("/events/:id", handleGetEvent(svcs))
	r.GET("/events/:id/availability", handleGetAvailability(svcs))
	r.GET("/events/:id/seats", handleListEventSeats(svcs))

	r.POST("/events/:id/holds", handleCreateHold(svcs, idem))

	r.POST("/orders/confirm", handleConfirmOrder(svcs))
	r.GET("/orders/:id", handleGetOrder(svcs))

	// Admin-API
	// TODO: add admin middleware
	admin := r.Group("/admin")
	{
		admin.POST("/venues", handleCreateVenue(svcs))
		admin.POST("/venues/:id/seats", handleBatchCreateSeats(svcs))
		admin.POST("/events", handleCreateEvent(svcs))
	}

	return r
}

// --- Handlers with Swagger annotations ---

// @Summary  Get event
// @Param    id  path  int  true  "Event ID"
// @Success  200  {object}  domain.Event
// @Failure  404  {object}  ErrorResponse
// @Router   /events/{id} [get]
func handleGetEvent(svcs *service.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID, ok := parseInt64Param(c, "id")
		if !ok {
			return
		}
		e, err := svcs.Query.GetEvent(c.Request.Context(), eventID)
		if err != nil {
			respondErr(c, err)
			return
		}
		// ETag + Cache-Control 60s
		writeJSONWithCache(c, http.StatusOK, e, "public, max-age=60", true)
	}
}

// @Summary  Get availability counters
// @Param    id  path  int  true  "Event ID"
// @Success  200  {object}  domain.EventCounts
// @Router   /events/{id}/availability [get]
func handleGetAvailability(svcs *service.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID, ok := parseInt64Param(c, "id")
		if !ok {
			return
		}
		cnt, err := svcs.Query.CountsByStatus(c.Request.Context(), eventID)
		if err != nil {
			respondErr(c, err)
			return
		}
		// ETag + Cache-Control 15s
		writeJSONWithCache(c, http.StatusOK, cnt, "public, max-age=15", true)
	}
}

// @Summary  List event seats
// @Param    id     path   int     true  "Event ID"
// @Param    only   query  string  false "available"
// @Param    limit  query  int     false "page size"
// @Param    offset query  int     false "offset"
// @Success  200  {array}   domain.SeatWithStatus
// @Router   /events/{id}/seats [get]
func handleListEventSeats(svcs *service.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID, ok := parseInt64Param(c, "id")
		if !ok {
			return
		}
		onlyAvailable := false
		if c.Query("only") == "available" ||
			c.Query("only_available") == "true" ||
			c.Query("onlyAvailable") == "true" {
			onlyAvailable = true
		}
		limit := parseIntDefault(c.Query("limit"), 100)
		offset := parseIntDefault(c.Query("offset"), 0)

		seats, err := svcs.Query.ListEventSeats(
			c.Request.Context(),
			eventID,
			onlyAvailable,
			limit,
			offset,
		)
		if err != nil {
			respondErr(c, err)
			return
		}
		// ETag + Cache-Control 15s (для списків — коротше)
		writeJSONWithCache(c, http.StatusOK, seats, "public, max-age=15", true)
	}
}

// @Summary  Create hold (idempotent)
// @Param    id  path  int  true  "Event ID"
// @Param    req body  CreateHoldRequest true "payload"
// @Header   201 {string} Idempotency-Key "echo"
// @Success  201 {object} CreateHoldResponse
// @Failure  400 {object} ErrorResponse
// @Failure  409 {object} ErrorResponse "seats unavailable / idem in progress"
// @Failure  429 {object} ErrorResponse "rate limited"
// @Router   /events/{id}/holds [post]
func handleCreateHold(
	svcs *service.Services,
	idem *redisrepo.IdempotencyStore,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID, ok := parseInt64Param(c, "id")
		if !ok {
			return
		}
		var req CreateHoldRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			badRequest(c, err.Error())
			return
		}

		idemKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
		var idemStorageKey string
		if idem != nil && idemKey != "" {
			idemStorageKey = redisrepo.KeyIdemHold(eventID, idemKey)

			if payload, ok, _ := idem.GetResult(
				c.Request.Context(),
				idemStorageKey,
			); ok {
				c.Header("Idempotency-Key", idemKey)
				c.Data(
					http.StatusCreated,
					"application/json; charset=utf-8",
					[]byte(payload),
				)
				return
			}

			locked, err := idem.AcquireLock(
				c.Request.Context(),
				idemStorageKey,
				60*time.Second,
			)
			if err != nil {
				respondErr(c, err)
				return
			}
			if !locked {
				if payload, ok, _ := idem.GetResult(
					c.Request.Context(),
					idemStorageKey,
				); ok {
					c.Header("Idempotency-Key", idemKey)
					c.Data(
						http.StatusCreated,
						"application/json; charset=utf-8",
						[]byte(payload),
					)
					return
				}
				c.Header("Retry-After", "1")
				c.JSON(
					http.StatusConflict,
					ErrorResponse{Error: "idempotency key in progress"},
				)
				return
			}
		}

		ttl := time.Duration(req.TTLSec) * time.Second
		rlKey := "ip:" + c.ClientIP()

		holdID, err := svcs.Reservation.CreateHold(
			c.Request.Context(),
			req.UserID,
			eventID,
			req.SeatIDs,
			ttl,
			rlKey,
		)
		if err != nil {
			if idemStorageKey != "" && idem != nil {
				_ = idem.Release(c.Request.Context(), idemStorageKey)
			}
			if isRateLimitedErr(err) {
				c.Header("Retry-After", "60")
				c.JSON(
					http.StatusTooManyRequests,
					ErrorResponse{Error: err.Error()},
				)
				return
			}
			respondErr(c, err)
			return
		}

		resp := CreateHoldResponse{HoldID: holdID.String()}

		if idemStorageKey != "" && idem != nil {
			b, _ := json.Marshal(resp)
			_ = idem.SaveResult(c.Request.Context(), idemStorageKey, string(b))
			c.Header("Idempotency-Key", idemKey)
		}

		c.JSON(http.StatusCreated, resp)
	}
}

// @Summary  Confirm order
// @Param    req body  ConfirmOrderRequest true "payload"
// @Success  201 {object} ConfirmOrderResponse
// @Failure  409 {object} ErrorResponse
// @Router   /orders/confirm [post]
func handleConfirmOrder(svcs *service.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ConfirmOrderRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			badRequest(c, err.Error())
			return
		}
		hid, err := uuid.Parse(req.HoldID)
		if err != nil {
			badRequest(c, "invalid hold_id")
			return
		}
		orderID, eventID, err := svcs.Reservation.Confirm(
			c.Request.Context(),
			hid,
			req.TotalCents,
		)
		if err != nil {
			respondErr(c, err)
			return
		}
		c.JSON(http.StatusCreated, ConfirmOrderResponse{
			OrderID: orderID.String(),
			EventID: eventID,
		})
	}
}

// @Summary  Get order with tickets
// @Param    id  path  string  true  "Order ID (uuid)"
// @Success  200 {object} domain.OrderWithTickets
// @Router   /orders/{id} [get]
func handleGetOrder(svcs *service.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		orderID := c.Param("id")
		o, err := svcs.Orders.GetOrderWithTickets(
			c.Request.Context(),
			orderID,
		)
		if err != nil {
			respondErr(c, err)
			return
		}
		c.JSON(http.StatusOK, o)
	}
}

// @Summary  Create venue
// @Param    req body  CreateVenueRequest true "payload"
// @Success  201 {object} CreateVenueResponse
// @Router   /admin/venues [post]
func handleCreateVenue(svcs *service.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateVenueRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			badRequest(c, err.Error())
			return
		}
		id, err := svcs.Admin.CreateVenue(
			c.Request.Context(),
			req.Name,
			req.SeatingScheme,
		)
		if err != nil {
			respondErr(c, err)
			return
		}
		c.JSON(http.StatusCreated, CreateVenueResponse{VenueID: id})
	}
}

// @Summary  Batch create seats
// @Param    id  path  int  true  "Venue ID"
// @Param    req body  BatchCreateSeatsRequest true "payload"
// @Success  201 {object} map[string]int
// @Router   /admin/venues/{id}/seats [post]
func handleBatchCreateSeats(svcs *service.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		venueID, ok := parseInt64Param(c, "id")
		if !ok {
			return
		}
		var req BatchCreateSeatsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			badRequest(c, err.Error())
			return
		}
		var seats []domain.Seat
		for _, s := range req.Seats {
			seats = append(seats, domain.Seat{
				VenueID: venueID,
				Section: s.Section,
				Row:     s.Row,
				Number:  s.Number,
			})
		}
		if err := svcs.Admin.BatchCreateSeats(
			c.Request.Context(),
			venueID,
			seats,
		); err != nil {
			respondErr(c, err)
			return
		}
		c.JSON(http.StatusCreated, gin.H{"created": len(seats)})
	}
}

// @Summary  Create event and init seats
// @Param    req body  CreateEventRequest true "payload"
// @Success  201 {object} CreateEventResponse
// @Router   /admin/events [post]
func handleCreateEvent(svcs *service.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateEventRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			badRequest(c, err.Error())
			return
		}
		starts, err := parseRFC3339(req.StartsAt)
		if err != nil {
			badRequest(c, "invalid starts_at (RFC3339)")
			return
		}
		ends, err := parseRFC3339(req.EndsAt)
		if err != nil {
			badRequest(c, "invalid ends_at (RFC3339)")
			return
		}
		id, err := svcs.Admin.CreateEventWithInit(
			c.Request.Context(),
			req.VenueID,
			req.Title,
			starts,
			ends,
		)
		if err != nil {
			respondErr(c, err)
			return
		}
		c.JSON(http.StatusCreated, CreateEventResponse{EventID: id})
	}
}

// --- Helpers ---

func parseInt64Param(c *gin.Context, name string) (int64, bool) {
	s := c.Param(name)
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		badRequest(c, "invalid "+name)
		return 0, false
	}
	return v, true
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func badRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, ErrorResponse{Error: msg})
}

func isRateLimitedErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "rate limited")
}

func respondErr(c *gin.Context, err error) {
	if err == nil {
		c.Status(http.StatusNoContent)
		return
	}

	switch {
	// admin service
	case errors.Is(err, admin.ErrEventConflict):
		c.JSON(http.StatusConflict, ErrorResponse{Error: "event conflict"})
		return
	case errors.Is(err, admin.ErrSeatsConflict):
		c.JSON(http.StatusConflict, ErrorResponse{Error: "seats conflict"})
		return
	case errors.Is(err, admin.ErrVenueConflict):
		c.JSON(http.StatusConflict, ErrorResponse{Error: "venue conflict"})
		return
	case errors.Is(err, admin.ErrFailedToInitEventSeats):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "event or venue does not exist"})
		return
	// orders service
	case errors.Is(err, orders.ErrOrderNotFound):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "order not found"})
		return
	// query service
	case errors.Is(err, query.ErrEventNotFound):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "event not found"})
		return
	case errors.Is(err, query.ErrOrderNotFound):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "order not found"})
		return
	// reservation service
	case errors.Is(err, reservation.ErrEventNotFound):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "event not found"})
		return
	case errors.Is(err, reservation.ErrHoldConflict):
		c.JSON(http.StatusConflict, ErrorResponse{Error: "hold conflict"})
		return
	case errors.Is(err, reservation.ErrHoldExpired):
		c.JSON(http.StatusConflict, ErrorResponse{Error: "hold expired"})
		return
	case errors.Is(err, reservation.ErrHoldNotFound):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "hold not found"})
		return
	case errors.Is(err, reservation.ErrSeatsUnavailable):
		c.JSON(http.StatusConflict, ErrorResponse{Error: "seats unavailable"})
		return
	}
}
