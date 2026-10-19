package linkhealth

import (
	"context"
	"errors"
	"testing"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/linkcheck"
)

type fakeChecker struct{}

func (fakeChecker) CheckMany(_ context.Context, urls []string, _ int) []linkcheck.BatchItem {
	return nil
}

func TestNewServiceNormalizesConcurrency(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, linkcheck.DefaultMaxConcurrency},
		{-1, linkcheck.DefaultMaxConcurrency},
		{2, 2},
		{100, linkcheck.MaxConcurrencyLimit},
	}
	for _, tc := range cases {
		s := NewService(nil, fakeChecker{}, tc.in)
		if s.Concurrency() != tc.want {
			t.Fatalf("concurrency(%d)=%d，期望 %d", tc.in, s.Concurrency(), tc.want)
		}
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name       string
		res        *linkcheck.Result
		err        error
		wantStatus string
		wantCode   int
	}{
		{"ok", &linkcheck.Result{Status: "ok", StatusCode: 200, FinalURL: "https://a"}, nil, "ok", 200},
		{"broken", &linkcheck.Result{Status: "broken", StatusCode: 404}, nil, "broken", 404},
		{"redirected", &linkcheck.Result{Status: "redirected", StatusCode: 200}, nil, "redirected", 200},
		{"unsafe", nil, linkcheck.ErrUnsafeURL, domain.LinkStatusBlocked, 0},
		{"network", nil, errors.New("dial tcp timeout"), domain.LinkStatusError, 0},
		{"empty status with result", &linkcheck.Result{StatusCode: 0}, errors.New("boom"), domain.LinkStatusError, 0},
	}
	for _, tc := range cases {
		status, code, _ := classify(tc.res, tc.err)
		if status != tc.wantStatus || code != tc.wantCode {
			t.Fatalf("%s: classify=%s/%d，期望 %s/%d", tc.name, status, code, tc.wantStatus, tc.wantCode)
		}
	}
}

func TestConfirmRejectsBadAction(t *testing.T) {
	s := NewService(nil, fakeChecker{}, 2)
	for _, action := range []string{"", "nope", domain.ReviewPending} {
		if _, err := s.Confirm(nil, "id", action, "admin", ""); !errors.Is(err, ErrBadAction) {
			t.Fatalf("动作 %q 应被拒绝，实际 %v", action, err)
		}
	}
}

func TestActionStateMapping(t *testing.T) {
	cases := map[string]string{
		"confirm_broken": domain.ReviewConfirmedBroken,
		"confirm_ok":     domain.ReviewConfirmedOK,
		"ignore":         domain.ReviewIgnored,
	}
	for action, want := range cases {
		got, ok := actionState[action]
		if !ok || got != want {
			t.Fatalf("动作 %q 应映射到 %q，实际 %q/%v", action, want, got, ok)
		}
	}
}

func TestNormalizePage(t *testing.T) {
	cases := []struct{ page, size, wantPage, wantSize int }{
		{0, 0, 1, 20},
		{-3, -1, 1, 20},
		{2, 50, 2, 50},
		{1, 999, 1, 100},
	}
	for _, tc := range cases {
		p, s := normalizePage(tc.page, tc.size)
		if p != tc.wantPage || s != tc.wantSize {
			t.Fatalf("normalizePage(%d,%d)=%d,%d，期望 %d,%d", tc.page, tc.size, p, s, tc.wantPage, tc.wantSize)
		}
	}
}
