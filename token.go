package main

import (
	"crypto/rand"
	"crypto/subtle"
	"log"
	"math/big"
	"sync"
	"time"
)

const tokenChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// TokenManager generates, rotates, and validates access tokens.
type TokenManager struct {
	mu      sync.Mutex
	current string
	enabled bool
}

// NewTokenManager creates a TokenManager. If ttl <= 0, token auth is
// disabled (Validate always returns true). Otherwise the token rotates
// every ttl and is printed to stdout at generation.
func NewTokenManager(ttl time.Duration) *TokenManager {
	tm := &TokenManager{enabled: ttl > 0}
	if !tm.enabled {
		return tm
	}

	tm.rotate()

	if ttl > 0 {
		go tm.rotator(ttl)
	}

	return tm
}

func (tm *TokenManager) rotator(ttl time.Duration) {
	ticker := time.NewTicker(ttl)
	defer ticker.Stop()
	for range ticker.C {
		tm.rotate()
	}
}

func (tm *TokenManager) rotate() {
	tm.mu.Lock()
	tm.current = generateToken()
	tm.mu.Unlock()
	log.Printf("puretty token: %s", tm.current)
}

// Validate returns true when token auth is disabled or when token
// matches the current token in constant time; false otherwise.
func (tm *TokenManager) Validate(token string) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if !tm.enabled {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(tm.current)) == 1
}

// generateToken produces an 8-character alphanumeric token using
// crypto/rand for uniform distribution over [a-zA-Z0-9].
func generateToken() string {
	const n = 8
	chars := []byte(tokenChars)
	limit := big.NewInt(int64(len(chars)))
	out := make([]byte, n)
	for i := range n {
		idx, err := rand.Int(rand.Reader, limit)
		if err != nil {
			log.Fatalf("token: crypto/rand int failed: %v", err)
		}
		out[i] = chars[idx.Int64()]
	}
	return string(out)
}
