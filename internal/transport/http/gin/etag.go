package httpgin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// writeJSONWithCache — writes a JSON response with ETag/Cache-Control.
// If If-None-Match matches the current ETag — returns 304.
func writeJSONWithCache(
	c *gin.Context,
	status int,
	v any,
	cacheControl string,
	weak bool,
) {
	b, err := json.Marshal(v)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	sum := sha256.Sum256(b)
	tag := `"` + hex.EncodeToString(sum[:]) + `"`
	if weak {
		tag = "W/" + tag
	}
	inm := c.GetHeader("If-None-Match")
	c.Header("ETag", tag)
	if cacheControl != "" {
		c.Header("Cache-Control", cacheControl)
	}
	if inm == tag {
		c.Status(http.StatusNotModified)
		return
	}
	c.Data(status, "application/json; charset=utf-8", b)
}
