package health_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"{{ cookiecutter.module_path }}/internal/health"
)

type fake struct {
	name string
	err  error
	wait time.Duration
}

func (f fake) Name() string { return f.name }

func (f fake) Check(ctx context.Context) error {
	select {
	case <-time.After(f.wait):
		return f.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestNoCheckersIsUp(t *testing.T) {
	rep := health.NewRegistry().Run(t.Context())
	assert.True(t, rep.Up())
	assert.Empty(t, rep.Checks)
	assert.NotNil(t, rep.Checks, "serialized as [] not null")
}

func TestReportSortedAndDown(t *testing.T) {
	rep := health.NewRegistry(
		health.WithChecker(fake{name: "redis", err: errors.New("refused")}),
		health.WithChecker(fake{name: "postgres"}),
	).Run(t.Context())

	assert.Equal(t, health.StatusDown, rep.Status)
	assert.Equal(t, []string{"postgres", "redis"}, []string{rep.Checks[0].Name, rep.Checks[1].Name})
	assert.Equal(t, health.StatusUp, rep.Checks[0].Status)
	assert.Equal(t, "refused", rep.Checks[1].Error)
}

func TestTimeout(t *testing.T) {
	rep := health.NewRegistry(
		health.WithTimeout(20*time.Millisecond),
		health.WithChecker(fake{name: "slow", wait: time.Second}),
	).Run(t.Context())

	assert.Equal(t, health.StatusDown, rep.Status)
	assert.Equal(t, context.DeadlineExceeded.Error(), rep.Checks[0].Error)
	assert.Less(t, rep.Checks[0].LatencyMS, int64(500))
}
