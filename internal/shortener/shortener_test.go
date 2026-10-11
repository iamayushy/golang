package shortener

import (
	"context"
	"errors"
	"testing"
)

func TestShortenAndResolve(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryRepository())

	link, err := svc.Shorten(ctx, "https://example.com/some/long/path")
	if err != nil {
		t.Fatalf("Shorten: %v", err)
	}
	if len(link.Code) != codeLength {
		t.Fatalf("code length = %d, want %d", len(link.Code), codeLength)
	}

	got, err := svc.Resolve(ctx, link.Code)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.URL != link.URL || got.Clicks != 1 {
		t.Fatalf("Resolve = %+v, want url %q with 1 click", got, link.URL)
	}
}

func TestShortenRejectsInvalidURLs(t *testing.T) {
	svc := NewService(NewMemoryRepository())
	for _, raw := range []string{"", "example.com", "ftp://example.com", "http://", "javascript:alert(1)"} {
		if _, err := svc.Shorten(context.Background(), raw); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("Shorten(%q) err = %v, want ErrInvalidURL", raw, err)
		}
	}
}

type collidingRepo struct {
	*MemoryRepository
	collisions int
}

func (c *collidingRepo) Create(ctx context.Context, link Link) error {
	if c.collisions > 0 {
		c.collisions--
		return ErrCodeExists
	}
	return c.MemoryRepository.Create(ctx, link)
}

func TestShortenRetriesOnCollision(t *testing.T) {
	svc := NewService(&collidingRepo{MemoryRepository: NewMemoryRepository(), collisions: 2})
	if _, err := svc.Shorten(context.Background(), "https://example.com"); err != nil {
		t.Fatalf("Shorten: %v", err)
	}

	svc = NewService(&collidingRepo{MemoryRepository: NewMemoryRepository(), collisions: maxAttempts})
	if _, err := svc.Shorten(context.Background(), "https://example.com"); err == nil {
		t.Fatal("Shorten succeeded despite exhausting attempts")
	}
}
