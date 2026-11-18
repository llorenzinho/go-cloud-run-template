package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConnURLEscapes(t *testing.T) {
	c := Conn{Host: "db.local", Port: 5432, User: "app", Password: "p@ss:/w?rd", Database: "app", SSLMode: "require"}
	assert.Equal(t, "postgres://app:p%40ss%3A%2Fw%3Frd@db.local:5432/app?sslmode=require", c.URL())
}

func TestWithTimeout(t *testing.T) {
	ctx, cancel := withTimeout(context.Background(), 0)
	defer cancel()
	_, ok := ctx.Deadline()
	assert.False(t, ok, "0 disables the timeout")

	ctx, cancel = withTimeout(context.Background(), time.Second)
	defer cancel()
	dl, ok := ctx.Deadline()
	assert.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(time.Second), dl, 100*time.Millisecond)

	short, cancelShort := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancelShort()
	ctx, cancel = withTimeout(short, time.Hour)
	defer cancel()
	assert.Equal(t, short, ctx, "an earlier deadline is kept")

	long, cancelLong := context.WithTimeout(context.Background(), time.Hour)
	defer cancelLong()
	ctx, cancel = withTimeout(long, time.Second)
	defer cancel()
	dl, _ = ctx.Deadline()
	assert.WithinDuration(t, time.Now().Add(time.Second), dl, 100*time.Millisecond, "a later deadline is shortened")
}
