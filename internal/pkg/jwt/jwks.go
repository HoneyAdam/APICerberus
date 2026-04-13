package jwt

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

var ErrJWKSKeyNotFound = errors.New("jwks key not found")

// JWKSClient fetches JWKS documents and caches parsed keys.
type JWKSClient struct {
	url        string
	ttl        time.Duration
	httpClient *http.Client

	now func() time.Time

	mu      sync.RWMutex
	rsaKeys map[string]*rsa.PublicKey
	ecKeys  map[string]*ecdsa.PublicKey
	fetched time.Time
}

// NewJWKSClient creates a JWKS client with cache TTL (defaults to 1h).
func NewJWKSClient(url string, ttl time.Duration) *JWKSClient {
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &JWKSClient{
		url:        strings.TrimSpace(url),
		ttl:        ttl,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		now:        time.Now,
		rsaKeys:    make(map[string]*rsa.PublicKey),
		ecKeys:     make(map[string]*ecdsa.PublicKey),
	}
}

// GetRSAKey resolves an RSA public key by kid.
func (c *JWKSClient) GetRSAKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if c == nil {
		return nil, errors.New("jwks client is nil")
	}
	kid = strings.TrimSpace(kid)
	if !c.isFresh() {
		if err := c.refresh(ctx); err != nil {
			if key, ok := c.lookupRSA(kid); ok {
				return key, nil
			}
			return nil, err
		}
	}
	if key, ok := c.lookupRSA(kid); ok {
		return key, nil
	}
	return nil, ErrJWKSKeyNotFound
}

// GetECDSAKey resolves an ECDSA public key by kid.
func (c *JWKSClient) GetECDSAKey(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	if c == nil {
		return nil, errors.New("jwks client is nil")
	}
	kid = strings.TrimSpace(kid)
	if !c.isFresh() {
		if err := c.refresh(ctx); err != nil {
			if key, ok := c.lookupECDSA(kid); ok {
				return key, nil
			}
			return nil, err
		}
	}
	if key, ok := c.lookupECDSA(kid); ok {
		return key, nil
	}
	return nil, ErrJWKSKeyNotFound
}

func (c *JWKSClient) isFresh() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.fetched.IsZero() {
		return false
	}
	return c.now().Sub(c.fetched) < c.ttl
}

func (c *JWKSClient) lookupRSA(kid string) (*rsa.PublicKey, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if kid != "" {
		key, ok := c.rsaKeys[kid]
		return key, ok
	}
	if len(c.rsaKeys) == 1 {
		for _, key := range c.rsaKeys {
			return key, true
		}
	}
	return nil, false
}

func (c *JWKSClient) lookupECDSA(kid string) (*ecdsa.PublicKey, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if kid != "" {
		key, ok := c.ecKeys[kid]
		return key, ok
	}
	if len(c.ecKeys) == 1 {
		for _, key := range c.ecKeys {
			return key, true
		}
	}
	return nil, false
}

func (c *JWKSClient) refresh(ctx context.Context) error {
	if strings.TrimSpace(c.url) == "" {
		return errors.New("jwks url is empty")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("jwks request failed: status %d", resp.StatusCode)
	}

	// Limit body size to prevent memory exhaustion attacks (max 1MB)
	var doc JWKS
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return err
	}

	rsaKeys := make(map[string]*rsa.PublicKey)
	ecKeys := make(map[string]*ecdsa.PublicKey)
	for _, jwk := range doc.Keys {
		kid := strings.TrimSpace(jwk.Kid)
		switch strings.ToUpper(jwk.Kty) {
		case "RSA":
			pub, err := ParseRSAPublicKeyFromJWK(jwk)
			if err == nil && pub != nil {
				rsaKeys[kid] = pub
			}
		case "EC":
			pub, err := ParseECDSAPublicKeyFromJWK(jwk)
			if err == nil && pub != nil {
				ecKeys[kid] = pub
			}
		}
	}
	if len(rsaKeys) == 0 && len(ecKeys) == 0 {
		return errors.New("jwks has no usable keys")
	}

	c.mu.Lock()
	c.rsaKeys = rsaKeys
	c.ecKeys = ecKeys
	c.fetched = c.now()
	c.mu.Unlock()
	return nil
}
