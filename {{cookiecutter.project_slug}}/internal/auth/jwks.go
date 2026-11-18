package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// emptyRetryInterval bounds refreshes while no key has ever been loaded (the
// IdP was unreachable at startup): readiness probes and requests retry often.
const emptyRetryInterval = 10 * time.Second

// jwksCache holds the keys of a JWKS URL: fetched at startup, refreshed in the
// background and on an unknown `kid` (rate limited), cleared of revoked keys
// on every refresh.
type jwksCache struct {
	v      *Verifier
	store  jwkset.Storage
	kf     keyfunc.Keyfunc
	loaded atomic.Bool

	mu          sync.Mutex
	lastAttempt time.Time
}

func newJWKSCache(ctx context.Context, v *Verifier) (*jwksCache, error) {
	store := jwkset.NewMemoryStorage()
	kf, err := keyfunc.New(keyfunc.Options{
		Ctx:     ctx,
		Storage: store,
		// Never verify a signature with an encryption key.
		UseWhitelist: []jwkset.USE{jwkset.UseSig, ""},
	})
	if err != nil {
		return nil, fmt.Errorf("jwks: %w", err)
	}
	c := &jwksCache{v: v, store: store, kf: kf}

	c.mu.Lock()
	c.lastAttempt = time.Now()
	c.mu.Unlock()
	if err := c.refresh(ctx); err != nil {
		v.log.WarnContext(ctx, "jwks fetch failed, retrying on demand", "url", v.jwksURL, "error", err)
	}
	go c.run(ctx)
	return c, nil
}

// run refreshes the keys every jwksRefresh until ctx is done.
func (c *jwksCache) run(ctx context.Context) {
	t := time.NewTicker(c.v.jwksRefresh)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := c.refresh(ctx); err != nil {
				c.v.log.WarnContext(ctx, "jwks refresh failed", "url", c.v.jwksURL, "error", err)
			}
		}
	}
}

func (c *jwksCache) refresh(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.v.jwksFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.v.jwksURL, nil)
	if err != nil {
		return err
	}
	res, err := c.v.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", c.v.jwksURL, res.StatusCode)
	}
	var set jwkset.JWKSMarshal
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&set); err != nil {
		return fmt.Errorf("decode jwks: %w", err)
	}
	keys := make([]jwkset.JWK, 0, len(set.Keys))
	for _, m := range set.Keys {
		k, err := jwkset.NewJWKFromMarshal(m, jwkset.JWKMarshalOptions{}, jwkset.JWKValidateOptions{})
		if errors.Is(err, jwkset.ErrUnsupportedKey) {
			continue
		}
		if err != nil {
			return fmt.Errorf("jwk %q: %w", m.KID, err)
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return errors.New("jwks has no usable key")
	}
	if err := c.store.KeyReplaceAll(ctx, keys); err != nil {
		return err
	}
	c.loaded.Store(true)
	return nil
}

// tryRefresh refreshes unless another attempt happened within the allowed
// interval; it reports whether a refresh succeeded.
func (c *jwksCache) tryRefresh(ctx context.Context) bool {
	interval := c.v.unknownKIDInterval
	if !c.loaded.Load() {
		interval = min(interval, emptyRetryInterval)
	}
	c.mu.Lock()
	if time.Since(c.lastAttempt) < interval {
		c.mu.Unlock()
		return false
	}
	c.lastAttempt = time.Now()
	c.mu.Unlock()
	if err := c.refresh(ctx); err != nil {
		c.v.log.WarnContext(ctx, "jwks refresh failed", "url", c.v.jwksURL, "error", err)
		return false
	}
	return true
}

// keyfunc returns the key of the token's kid, refreshing once on a miss
// (key rotation at the IdP).
func (c *jwksCache) keyfunc(t *jwt.Token) (any, error) {
	ctx := context.Background()
	key, err := c.kf.KeyfuncCtx(ctx)(t)
	if err != nil && errors.Is(err, jwkset.ErrKeyNotFound) && c.tryRefresh(ctx) {
		return c.kf.KeyfuncCtx(ctx)(t)
	}
	return key, err
}

// Name implements health.Checker.
func (c *jwksCache) Name() string { return "auth_jwks" }

// Check implements health.Checker: down until the keys have been loaded once.
func (c *jwksCache) Check(ctx context.Context) error {
	if c.loaded.Load() || c.tryRefresh(ctx) {
		return nil
	}
	return errors.New("jwks not loaded yet")
}
