// Package middleware implements the cross-cutting request pipeline: request
// correlation, structured logging, panic recovery, CORS and the authorization
// chain of §C2.
package middleware

import (
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/config"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/security"
)

// Context keys shared with the controllers.
const (
	CtxRequestID = "requestId"
	CtxUserID    = "userId"
	CtxCompanyID = "companyId"
)

// RequestID assigns or adopts a correlation id and puts it on the context, the
// response header and every log line (§D5).
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-Id")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Set(CtxRequestID, requestID)
		c.Header("X-Request-Id", requestID)

		ctx := security.WithRequestID(c.Request.Context(), requestID)
		ctx = security.WithIPAddress(ctx, c.ClientIP())
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// Logger emits one structured line per request with the fields §D5 asks for.
func Logger(log zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		event := log.Info()
		if c.Writer.Status() >= http.StatusInternalServerError {
			event = log.Error()
		} else if c.Writer.Status() >= http.StatusBadRequest {
			event = log.Warn()
		}

		event = event.
			Str("requestId", c.GetString(CtxRequestID)).
			Str("method", c.Request.Method).
			Str("route", c.FullPath()).
			Int("status", c.Writer.Status()).
			Int64("latencyMs", time.Since(start).Milliseconds()).
			Str("ip", c.ClientIP())

		if userID := c.GetInt64(CtxUserID); userID != 0 {
			event = event.Int64("userId", userID)
		}
		if companyID := c.GetInt64(CtxCompanyID); companyID != 0 {
			event = event.Int64("companyId", companyID)
		}
		event.Msg("request")
	}
}

// Recovery turns a panic into a 500 with the standard error envelope, keeping
// the stack in the log and out of the response.
func Recovery(log zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Error().
					Interface("panic", recovered).
					Str("requestId", c.GetString(CtxRequestID)).
					Str("route", c.FullPath()).
					Bytes("stack", stack()).
					Msg("panic recovered")

				Abort(c, apperrors.ErrInternal)
			}
		}()
		c.Next()
	}
}

// CORS restricts browser access to the configured origins (§C4).
func CORS(cfg config.CORSConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && slices.Contains(cfg.AllowedOrigins, origin) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers",
				"Authorization, Content-Type, X-Company-Id, X-Request-Id, Idempotency-Key")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Max-Age", "600")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// RequireJSON enforces the Content-Type rule of §C4 on state-changing calls.
func RequireJSON() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			if c.Request.ContentLength > 0 {
				contentType := c.ContentType()
				if contentType != "application/json" {
					Abort(c, apperrors.ErrBadRequest.
						Msgf("Content-Type must be application/json"))
					return
				}
			}
		}
		c.Next()
	}
}

// Abort writes the standard error envelope and stops the chain. Controllers
// share it through the Respond helper, so there is exactly one place that
// decides what an error looks like on the wire.
func Abort(c *gin.Context, err error) {
	appErr := apperrors.Wrap(err)
	c.AbortWithStatusJSON(appErr.HTTPStatus, gin.H{
		"success": false,
		"error": gin.H{
			"code":    appErr.Code,
			"message": appErr.Message,
			"details": appErr.Details,
		},
		"requestId": c.GetString(CtxRequestID),
	})
}
