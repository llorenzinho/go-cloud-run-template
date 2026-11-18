// Package health runs the readiness checks of the service dependencies.
package health

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"
)

// Statuses of a check and of the whole report.
const (
	StatusUp   = "up"
	StatusDown = "down"
)

// Checker is a dependency that can report whether it is usable.
type Checker interface {
	// Name identifies the check in the report, e.g. "postgres".
	Name() string
	// Check returns nil when the dependency is usable.
	Check(ctx context.Context) error
}

// Result is the outcome of one check.
type Result struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	LatencyMS int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

// Report is the outcome of every check. Status is down when any check is down.
type Report struct {
	Status string   `json:"status"`
	Checks []Result `json:"checks"`
}

// Up reports whether every check passed.
func (r Report) Up() bool { return r.Status == StatusUp }

// Registry holds the checkers.
type Registry struct {
	checkers []Checker
	timeout  time.Duration
}

// Option configures NewRegistry.
type Option func(*Registry)

// WithChecker adds c to the checks.
func WithChecker(c Checker) Option {
	return func(r *Registry) { r.checkers = append(r.checkers, c) }
}

// WithTimeout bounds each check (default 2s).
func WithTimeout(d time.Duration) Option {
	return func(r *Registry) { r.timeout = d }
}

// NewRegistry returns a registry of the given checkers.
func NewRegistry(opts ...Option) *Registry {
	r := &Registry{timeout: 2 * time.Second}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Run executes every check in parallel and returns the results sorted by name.
func (r *Registry) Run(ctx context.Context) Report {
	results := make([]Result, len(r.checkers))
	var wg sync.WaitGroup
	for i, c := range r.checkers {
		wg.Go(func() { results[i] = r.check(ctx, c) })
	}
	wg.Wait()

	slices.SortFunc(results, func(a, b Result) int { return cmp.Compare(a.Name, b.Name) })
	rep := Report{Status: StatusUp, Checks: results}
	for _, res := range results {
		if res.Status != StatusUp {
			rep.Status = StatusDown
		}
	}
	return rep
}

func (r *Registry) check(ctx context.Context, c Checker) Result {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	start := time.Now()
	err := c.Check(ctx)
	res := Result{Name: c.Name(), Status: StatusUp, LatencyMS: time.Since(start).Milliseconds()}
	if err != nil {
		res.Status, res.Error = StatusDown, err.Error()
	}
	return res
}
