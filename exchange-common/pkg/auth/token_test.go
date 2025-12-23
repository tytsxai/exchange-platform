package auth

import (
	"testing"
	"time"
)

func TestTokenManagerIssueAndVerify(t *testing.T) {
	manager, err := NewTokenManager("this-is-a-test-secret-with-32-bytes-min", 2*time.Hour)
	if err != nil {
		t.Fatalf("NewTokenManager error: %v", err)
	}

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	manager.clock = func() time.Time { return now }

	token, err := manager.Issue(12345)
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}

	userID, err := manager.Verify(token)
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	if userID != 12345 {
		t.Fatalf("expected userID=12345, got %d", userID)
	}
}

func TestTokenManagerExpired(t *testing.T) {
	manager, err := NewTokenManager("another-test-secret-with-32-bytes-min", time.Minute)
	if err != nil {
		t.Fatalf("NewTokenManager error: %v", err)
	}

	issueTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	manager.clock = func() time.Time { return issueTime }
	token, err := manager.Issue(100)
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}

	manager.clock = func() time.Time { return issueTime.Add(2 * time.Minute) }
	if _, err := manager.Verify(token); err != ErrTokenExpired {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
}

func TestTokenManagerInvalidSignature(t *testing.T) {
	manager, err := NewTokenManager("yet-another-test-secret-with-32-bytes", 10*time.Minute)
	if err != nil {
		t.Fatalf("NewTokenManager error: %v", err)
	}

	token, err := manager.Issue(999)
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}

	// Tamper token
	token += "deadbeef"

	if _, err := manager.Verify(token); err != ErrInvalidSignature {
		t.Fatalf("expected ErrInvalidSignature, got %v", err)
	}
}
