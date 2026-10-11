// Package shortener holds the URL shortener domain logic, independent of storage.
package shortener

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"time"
)

var (
	ErrNotFound   = errors.New("link not found")
	ErrCodeExists = errors.New("code already exists")
	ErrInvalidURL = errors.New("url must be an absolute http or https URL")
)

type Link struct {
	Code      string    `json:"code" bson:"code"`
	URL       string    `json:"url" bson:"url"`
	Clicks    int64     `json:"clicks" bson:"clicks"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

// Repository persists links. Create must return ErrCodeExists on a duplicate
// code; Get, Resolve and Delete must return ErrNotFound for unknown codes.
type Repository interface {
	Create(ctx context.Context, link Link) error
	Get(ctx context.Context, code string) (Link, error)
	// Resolve increments the click counter and returns the link.
	Resolve(ctx context.Context, code string) (Link, error)
	Delete(ctx context.Context, code string) error
}

const (
	codeAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	codeLength   = 7
	maxAttempts  = 5
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Shorten validates rawURL and stores it under a freshly generated code.
func (s *Service) Shorten(ctx context.Context, rawURL string) (Link, error) {
	if !validURL(rawURL) {
		return Link{}, ErrInvalidURL
	}
	for range maxAttempts {
		code, err := generateCode()
		if err != nil {
			return Link{}, err
		}
		link := Link{Code: code, URL: rawURL, CreatedAt: time.Now().UTC()}
		err = s.repo.Create(ctx, link)
		if errors.Is(err, ErrCodeExists) {
			continue
		}
		if err != nil {
			return Link{}, err
		}
		return link, nil
	}
	return Link{}, fmt.Errorf("could not generate a unique code after %d attempts", maxAttempts)
}

func (s *Service) Get(ctx context.Context, code string) (Link, error) {
	return s.repo.Get(ctx, code)
}

func (s *Service) Resolve(ctx context.Context, code string) (Link, error) {
	return s.repo.Resolve(ctx, code)
}

func (s *Service) Delete(ctx context.Context, code string) error {
	return s.repo.Delete(ctx, code)
}

func validURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func generateCode() (string, error) {
	b := make([]byte, codeLength)
	max := big.NewInt(int64(len(codeAlphabet)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = codeAlphabet[n.Int64()]
	}
	return string(b), nil
}
