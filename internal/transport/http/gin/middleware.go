package httpgin

import (
	"log/slog"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader("X-Request-ID")
		if reqID == "" {
			reqID = uuid.New().String()
		}

		c.Writer.Header().Set("X-Request-ID", reqID)
		c.Set("request_id", reqID)

		c.Next()
	}
}

func CORS() gin.HandlerFunc {
	cfg := cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{
			"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS",
		},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Accept",
			"Authorization",
			"X-Requested-With",
			"X-Request-ID",
			"Idempotency-Key",
			"If-None-Match",
		},
		ExposeHeaders: []string{
			"X-Request-ID",
			"ETag",
			"Cache-Control",
		},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}

	return cors.New(cfg)
}

func LoggingMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery
		c.Next()

		latency := time.Since(start)
		if raw != "" {
			path = path + "?" + raw
		}

		status := c.Writer.Status()
		reqID, _ := c.Get("request_id")

		attrs := []slog.Attr{
			slog.Int("status", status),
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.String("ip", c.ClientIP()),
			slog.String("ua", c.Request.UserAgent()),
			slog.Any("request_id", reqID),
			slog.Duration("latency", latency),
			slog.Int("bytes_out", c.Writer.Size()),
		}

		// convert []slog.Attr to []any for slog.Group variadic parameter
		anyAttrs := make([]any, len(attrs))
		for i := range attrs {
			anyAttrs[i] = attrs[i]
		}

		if len(c.Errors) > 0 {
			logger.Error("http", slog.Group("http", anyAttrs...))
		} else {
			logger.Info("http", slog.Group("http", anyAttrs...))
		}
	}
}
