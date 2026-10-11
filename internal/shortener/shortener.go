// Package shortener holds the URL shortener domain logic, independent of storage.
package shortener

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"time"
)

const (
	MinTTL     = 10 * time.Minute
	MaxTTL     = 30 * time.Minute
	DefaultTTL = MinTTL
)

var (
	ErrNotFound     = errors.New("link not found")
	ErrCodeExists   = errors.New("code already exists")
	ErrInvalidURL   = errors.New("url must be an absolute http or https URL")
	ErrInvalidTTL   = errors.New("ttl_minutes must be between 10 and 30")
	ErrInvalidToken = errors.New("invalid delete token")
)

type Link struct {
	Code            string    `json:"code" bson:"code"`
	URL             string    `json:"url" bson:"url"`
	Clicks          int64     `json:"clicks" bson:"clicks"`
	CreatedAt       time.Time `json:"created_at" bson:"created_at"`
	ExpiresAt       time.Time `json:"expires_at" bson:"expires_at"`
	DeleteTokenHash string    `json:"-" bson:"delete_token_hash"`
}

// Repository persists links. Create must return ErrCodeExists on a duplicate
// code; Get, Resolve and Delete must return ErrNotFound for unknown codes.
// Expiry is enforced by the Service, so repositories may return expired links.
type Repository interface {
	Create(ctx context.Context, link Link) error
	Get(ctx context.Context, code string) (Link, error)
	// Resolve increments the click counter and returns the link.
	Resolve(ctx context.Context, code string) (Link, error)
	Delete(ctx context.Context, code string) error
	// Stats reports how many links were ever created and how many expire after now.
	Stats(ctx context.Context, now time.Time) (Stats, error)
}

type Stats struct {
	TotalCreated int64 `json:"total_created"`
	Live         int64 `json:"live"`
}

const (
	codeAlphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	codeLength   = 6
	maxAttempts  = 5
)

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// Created is a freshly stored link plus the one-time token needed to delete it.
type Created struct {
	Link
	DeleteToken string
}

// Shorten validates rawURL and ttl and stores the link under a freshly generated code.
// A zero ttl means DefaultTTL.
func (s *Service) Shorten(ctx context.Context, rawURL string, ttl time.Duration) (Created, error) {
	if !validURL(rawURL) {
		return Created{}, ErrInvalidURL
	}
	if ttl == 0 {
		ttl = DefaultTTL
	}
	if ttl < MinTTL || ttl > MaxTTL {
		return Created{}, ErrInvalidTTL
	}
	token, err := randomString(24)
	if err != nil {
		return Created{}, err
	}
	now := s.now().UTC()
	for range maxAttempts {
		code, err := randomString(codeLength)
		if err != nil {
			return Created{}, err
		}
		link := Link{
			Code:            code,
			URL:             rawURL,
			CreatedAt:       now,
			ExpiresAt:       now.Add(ttl),
			DeleteTokenHash: hashToken(token),
		}
		err = s.repo.Create(ctx, link)
		if errors.Is(err, ErrCodeExists) {
			continue
		}
		if err != nil {
			return Created{}, err
		}
		return Created{Link: link, DeleteToken: token}, nil
	}
	return Created{}, fmt.Errorf("could not generate a unique code after %d attempts", maxAttempts)
}

func (s *Service) Get(ctx context.Context, code string) (Link, error) {
	return s.alive(s.repo.Get(ctx, code))
}

func (s *Service) Resolve(ctx context.Context, code string) (Link, error) {
	return s.alive(s.repo.Resolve(ctx, code))
}

// Delete removes a link if token matches the one issued when it was created.
func (s *Service) Delete(ctx context.Context, code, token string) error {
	link, err := s.Get(ctx, code)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(hashToken(token)), []byte(link.DeleteTokenHash)) != 1 {
		return ErrInvalidToken
	}
	return s.repo.Delete(ctx, code)
}

func (s *Service) Stats(ctx context.Context) (Stats, error) {
	return s.repo.Stats(ctx, s.now())
}

// TimeLeft reports how long until link expires, never negative.
func (s *Service) TimeLeft(link Link) time.Duration {
	return max(link.ExpiresAt.Sub(s.now()), 0)
}

// alive hides links past their expiry; the database removes them shortly after.
func (s *Service) alive(link Link, err error) (Link, error) {
	if err != nil {
		return Link{}, err
	}
	if !s.now().Before(link.ExpiresAt) {
		return Link{}, ErrNotFound
	}
	return link, nil
}

func validURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomString(n int) (string, error) {
	b := make([]byte, n)
	max := big.NewInt(int64(len(codeAlphabet)))
	for i := range b {
		r, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = codeAlphabet[r.Int64()]
	}
	return string(b), nil
}
