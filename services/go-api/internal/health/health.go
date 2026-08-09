package health

import (
	"context"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

type Check func(context.Context) error
type Dependency struct {
	Name  string
	Check Check
}
type Result struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}
type Response struct {
	Status string            `json:"status"`
	Checks map[string]Result `json:"checks"`
}
type Checker struct{ dependencies []Dependency }

func NewChecker(dependencies ...Dependency) *Checker { return &Checker{dependencies: dependencies} }

func (c *Checker) Check(ctx context.Context) Response {
	results := make(map[string]Result, len(c.dependencies))
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, dependency := range c.dependencies {
		dependency := dependency
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := Result{Status: "ok"}
			if dependency.Check == nil {
				result = Result{Status: "failed", Error: "dependency check is not configured"}
			} else if err := dependency.Check(ctx); err != nil {
				result = Result{Status: "failed", Error: "dependency is unavailable"}
			}
			mu.Lock()
			results[dependency.Name] = result
			mu.Unlock()
		}()
	}
	wg.Wait()
	response := Response{Status: "ready", Checks: results}
	for _, result := range results {
		if result.Status != "ok" {
			response.Status = "not_ready"
			break
		}
	}
	return response
}

func (c *Checker) Handler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		response := c.Check(ctx.Request.Context())
		status := http.StatusOK
		if response.Status != "ready" {
			status = http.StatusServiceUnavailable
		}
		ctx.JSON(status, response)
	}
}

func Healthz(ctx *gin.Context) { ctx.String(http.StatusOK, "ok\n") }
