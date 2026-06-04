package domain

import "testing"

func TestCanTransitionValidLifecycle(t *testing.T) {
	cases := []struct {
		from string
		to   string
	}{
		{StatusPending, StatusActive},
		{StatusActive, StatusSuspended},
		{StatusSuspended, StatusActive},
		{StatusActive, StatusDeregistered},
		{StatusSuspended, StatusDeregistered},
		{StatusActive, StatusActive}, // idempotent no-op
	}
	for _, tc := range cases {
		if !CanTransition(tc.from, tc.to) {
			t.Fatalf("expected transition %s -> %s to be valid", tc.from, tc.to)
		}
	}
}

func TestCanTransitionInvalidLifecycle(t *testing.T) {
	cases := []struct {
		from string
		to   string
	}{
		{StatusPending, StatusSuspended},
		{StatusPending, StatusDeregistered},
		{StatusDeregistered, StatusActive},
		{"UNKNOWN", StatusActive},
	}
	for _, tc := range cases {
		if CanTransition(tc.from, tc.to) {
			t.Fatalf("expected transition %s -> %s to be invalid", tc.from, tc.to)
		}
	}
}
