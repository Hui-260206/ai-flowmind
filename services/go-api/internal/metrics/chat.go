// Package metrics exposes bounded operational metrics for the mobile-facing
// synchronous chat workflow. It deliberately accepts no request identifiers or
// content, preventing accidental high-cardinality time series.
package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type ChatMetrics interface {
	ObserveChatRequest(outcome, code string, status int, duration time.Duration)
	ObserveGRPC(outcome string, duration time.Duration)
	IncAITimeout()
	IncMessagePersistFailure(stage string)
	IncRedisLockFailure(stage string)
}

type Collector struct {
	chatRequests      *prometheus.CounterVec
	chatFailures      *prometheus.CounterVec
	chatLatency       *prometheus.HistogramVec
	aiGRPCLatency     *prometheus.HistogramVec
	aiTimeouts        prometheus.Counter
	persistFailures   *prometheus.CounterVec
	redisLockFailures *prometheus.CounterVec
}

func New(registry *prometheus.Registry) *Collector {
	c := &Collector{
		chatRequests:      prometheus.NewCounterVec(prometheus.CounterOpts{Name: "chat_request_total", Help: "Completed synchronous chat send attempts."}, []string{"outcome", "status_code"}),
		chatFailures:      prometheus.NewCounterVec(prometheus.CounterOpts{Name: "chat_request_failed_total", Help: "Failed synchronous chat send attempts."}, []string{"error_code", "status_code"}),
		chatLatency:       prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "chat_request_latency", Help: "End-to-end synchronous chat send latency in seconds."}, []string{"outcome", "status_code"}),
		aiGRPCLatency:     prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "ai_grpc_latency", Help: "Go-to-Python AI gRPC latency in seconds."}, []string{"outcome"}),
		aiTimeouts:        prometheus.NewCounter(prometheus.CounterOpts{Name: "ai_timeout_total", Help: "AI gRPC calls that exceeded their deadline."}),
		persistFailures:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: "message_persist_failed_total", Help: "Chat message persistence failures."}, []string{"stage"}),
		redisLockFailures: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "redis_lock_failed_total", Help: "Redis session-lock coordination failures."}, []string{"stage"}),
	}
	registry.MustRegister(c.chatRequests, c.chatFailures, c.chatLatency, c.aiGRPCLatency, c.aiTimeouts, c.persistFailures, c.redisLockFailures)
	return c
}

func (c *Collector) ObserveChatRequest(outcome, code string, status int, d time.Duration) {
	statusText := strconv.Itoa(status)
	c.chatRequests.WithLabelValues(outcome, statusText).Inc()
	c.chatLatency.WithLabelValues(outcome, statusText).Observe(d.Seconds())
	if status >= 400 {
		c.chatFailures.WithLabelValues(code, statusText).Inc()
	}
}
func (c *Collector) ObserveGRPC(outcome string, d time.Duration) {
	c.aiGRPCLatency.WithLabelValues(outcome).Observe(d.Seconds())
}
func (c *Collector) IncAITimeout() { c.aiTimeouts.Inc() }
func (c *Collector) IncMessagePersistFailure(stage string) {
	c.persistFailures.WithLabelValues(stage).Inc()
}
func (c *Collector) IncRedisLockFailure(stage string) {
	c.redisLockFailures.WithLabelValues(stage).Inc()
}

type noop struct{}

func (noop) ObserveChatRequest(string, string, int, time.Duration) {}
func (noop) ObserveGRPC(string, time.Duration)                     {}
func (noop) IncAITimeout()                                         {}
func (noop) IncMessagePersistFailure(string)                       {}
func (noop) IncRedisLockFailure(string)                            {}
func Noop() ChatMetrics                                            { return noop{} }
