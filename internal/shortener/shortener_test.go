package shortener

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newTestService(repo Repository) (*Service, *time.Time) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc := NewService(repo)
	svc.now = func() time.Time { return now }
	return svc, &now
}

func TestShortenAndResolve(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(NewMemoryRepository())

	created, err := svc.Shorten(ctx, "https://example.com/some/long/path", 20*time.Minute)
	if err != nil {
		t.Fatalf("Shorten: %v", err)
	}
	if len(created.Code) != codeLength {
		t.Fatalf("code length = %d, want %d", len(created.Code), codeLength)
	}
	if got := created.ExpiresAt.Sub(created.CreatedAt); got != 20*time.Minute {
		t.Fatalf("ttl = %v, want 20m", got)
	}

	got, err := svc.Resolve(ctx, created.Code)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.URL != created.URL || got.Clicks != 1 {
		t.Fatalf("Resolve = %+v, want url %q with 1 click", got, created.URL)
	}
}

func TestExpiredLinksAreHidden(t *testing.T) {
	ctx := context.Background()
	svc, now := newTestService(NewMemoryRepository())

	created, err := svc.Shorten(ctx, "https://example.com", 0)
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(DefaultTTL - time.Second)
	if _, err := svc.Resolve(ctx, created.Code); err != nil {
		t.Fatalf("Resolve before expiry: %v", err)
	}
	if left := svc.TimeLeft(created.Link); left != time.Second {
		t.Fatalf("TimeLeft = %v, want 1s", left)
	}
	*now = now.Add(time.Second)
	if _, err := svc.Resolve(ctx, created.Code); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Resolve at expiry err = %v, want ErrNotFound", err)
	}
	if _, err := svc.Get(ctx, created.Code); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get at expiry err = %v, want ErrNotFound", err)
	}
}

func TestStatsCountExpiredLinksInTotalOnly(t *testing.T) {
	ctx := context.Background()
	svc, now := newTestService(NewMemoryRepository())
	if _, err := svc.Shorten(ctx, "https://example.com", 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Shorten(ctx, "https://example.com", 30*time.Minute); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(15 * time.Minute)
	st, err := svc.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st != (Stats{TotalCreated: 2, Live: 1}) {
		t.Fatalf("Stats = %+v, want 2 created, 1 live", st)
	}
}

func TestShortenRejectsInvalidInput(t *testing.T) {
	svc, _ := newTestService(NewMemoryRepository())
	for _, raw := range []string{"", "example.com", "ftp://example.com", "http://", "javascript:alert(1)"} {
		if _, err := svc.Shorten(context.Background(), raw, 0); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("Shorten(%q) err = %v, want ErrInvalidURL", raw, err)
		}
	}
	for _, ttl := range []time.Duration{time.Minute, 9 * time.Minute, 31 * time.Minute, -time.Minute} {
		if _, err := svc.Shorten(context.Background(), "https://example.com", ttl); !errors.Is(err, ErrInvalidTTL) {
			t.Errorf("Shorten(ttl %v) err = %v, want ErrInvalidTTL", ttl, err)
		}
	}
}

func TestDeleteRequiresToken(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(NewMemoryRepository())
	created, err := svc.Shorten(ctx, "https://example.com", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, created.Code, "wrong"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("Delete with wrong token err = %v, want ErrInvalidToken", err)
	}
	if err := svc.Delete(ctx, created.Code, created.DeleteToken); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, created.Code); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete err = %v, want ErrNotFound", err)
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
	svc, _ := newTestService(&collidingRepo{MemoryRepository: NewMemoryRepository(), collisions: 2})
	if _, err := svc.Shorten(context.Background(), "https://example.com", 0); err != nil {
		t.Fatalf("Shorten: %v", err)
	}

	svc, _ = newTestService(&collidingRepo{MemoryRepository: NewMemoryRepository(), collisions: maxAttempts})
	if _, err := svc.Shorten(context.Background(), "https://example.com", 0); err == nil {
		t.Fatal("Shorten succeeded despite exhausting attempts")
	}
}
