package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// GinMiddleware returns a Gin handler that records per-request metrics:
//   - increments nayoo_http_requests_total
//   - observes nayoo_http_request_duration_seconds
//   - tracks nayoo_http_inflight_requests (gauge inc/dec)
//
// The `path` label uses Gin's c.FullPath() (route template like
// "/users/:id") not the raw URL, so we don't blow up cardinality on
// dynamic paths. Routes that don't match any handler get the path
// label "<unmatched>".
//
// Init() must be called before this middleware is added.
func GinMiddleware() gin.HandlerFunc {
	if HTTPRequests == nil {
		panic("metrics: GinMiddleware() called before Init()")
	}
	return func(c *gin.Context) {
		HTTPInflight.WithLabelValues(serviceName).Inc()
		start := time.Now()

		c.Next()

		path := c.FullPath()
		if path == "" {
			path = "<unmatched>"
		}
		method := c.Request.Method
		code := strconv.Itoa(c.Writer.Status())
		elapsed := time.Since(start).Seconds()

		HTTPRequests.WithLabelValues(serviceName, method, path, code).Inc()
		HTTPDuration.WithLabelValues(serviceName, method, path).Observe(elapsed)
		HTTPInflight.WithLabelValues(serviceName).Dec()
	}
}
