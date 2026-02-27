package monitor

import (
	"testing"
	"time"

	"github.com/fabiant7t/eddie/internal/spec"
	"github.com/fabiant7t/eddie/internal/state"
)

func TestApplyCycleResultFailureAndRecoveryThresholds(t *testing.T) {
	current := state.SpecState{Status: state.StatusHealthy}

	// Needs 2 consecutive failures.
	next, transition := applyCycleResult(current, false, 2, 2)
	if transition != transitionNone {
		t.Fatalf("transition = %v, want %v", transition, transitionNone)
	}
	if next.Status != state.StatusHealthy || next.ConsecutiveFailures != 1 {
		t.Fatalf("unexpected state after first failure: %+v", next)
	}

	next, transition = applyCycleResult(next, false, 2, 2)
	if transition != transitionFailure {
		t.Fatalf("transition = %v, want %v", transition, transitionFailure)
	}
	if next.Status != state.StatusFailing {
		t.Fatalf("status = %v, want %v", next.Status, state.StatusFailing)
	}

	// Needs 2 consecutive successes to recover.
	next, transition = applyCycleResult(next, true, 2, 2)
	if transition != transitionNone {
		t.Fatalf("transition = %v, want %v", transition, transitionNone)
	}
	if next.Status != state.StatusFailing || next.ConsecutiveSuccesses != 1 {
		t.Fatalf("unexpected state after first success: %+v", next)
	}

	next, transition = applyCycleResult(next, true, 2, 2)
	if transition != transitionRecovery {
		t.Fatalf("transition = %v, want %v", transition, transitionRecovery)
	}
	if next.Status != state.StatusHealthy {
		t.Fatalf("status = %v, want %v", next.Status, state.StatusHealthy)
	}
}

func TestThresholdOrDefault(t *testing.T) {
	if got := thresholdOrDefault(0, 1); got != 1 {
		t.Fatalf("thresholdOrDefault(0,1) = %d, want %d", got, 1)
	}
	if got := thresholdOrDefault(-2, 1); got != 1 {
		t.Fatalf("thresholdOrDefault(-2,1) = %d, want %d", got, 1)
	}
	if got := thresholdOrDefault(3, 1); got != 3 {
		t.Fatalf("thresholdOrDefault(3,1) = %d, want %d", got, 3)
	}
}

func TestCyclesDefaultsToOne(t *testing.T) {
	parsedSpec := spec.Spec{
		HTTP: spec.HTTPSpec{
			Name: "s",
			Cycles: spec.SpecCycles{
				Failure: 0,
				Success: 0,
			},
		},
	}

	failureThreshold := thresholdOrDefault(parsedSpec.HTTP.Cycles.Failure, 1)
	successThreshold := thresholdOrDefault(parsedSpec.HTTP.Cycles.Success, 1)

	if failureThreshold != 1 {
		t.Fatalf("failureThreshold = %d, want 1", failureThreshold)
	}
	if successThreshold != 1 {
		t.Fatalf("successThreshold = %d, want 1", successThreshold)
	}
}

func TestResetStaleConsecutiveStateResetsCounters(t *testing.T) {
	now := time.Date(2026, 2, 27, 18, 0, 0, 0, time.UTC)
	current := state.SpecState{
		Status:               state.StatusHealthy,
		ConsecutiveFailures:  3,
		ConsecutiveSuccesses: 0,
		LastCycleAt:          now.Add(-3 * time.Minute),
	}

	next := resetStaleConsecutiveState(current, now, time.Minute)
	if next.ConsecutiveFailures != 0 {
		t.Fatalf("ConsecutiveFailures = %d, want 0", next.ConsecutiveFailures)
	}
	if next.ConsecutiveSuccesses != 0 {
		t.Fatalf("ConsecutiveSuccesses = %d, want 0", next.ConsecutiveSuccesses)
	}
}

func TestResetStaleConsecutiveStateKeepsFreshCounters(t *testing.T) {
	now := time.Date(2026, 2, 27, 18, 0, 0, 0, time.UTC)
	current := state.SpecState{
		Status:               state.StatusFailing,
		ConsecutiveFailures:  0,
		ConsecutiveSuccesses: 1,
		LastCycleAt:          now.Add(-90 * time.Second),
	}

	next := resetStaleConsecutiveState(current, now, time.Minute)
	if next.ConsecutiveFailures != current.ConsecutiveFailures {
		t.Fatalf("ConsecutiveFailures = %d, want %d", next.ConsecutiveFailures, current.ConsecutiveFailures)
	}
	if next.ConsecutiveSuccesses != current.ConsecutiveSuccesses {
		t.Fatalf("ConsecutiveSuccesses = %d, want %d", next.ConsecutiveSuccesses, current.ConsecutiveSuccesses)
	}
}

func TestHasStateChanged(t *testing.T) {
	base := state.SpecState{
		Status:               state.StatusHealthy,
		ConsecutiveFailures:  1,
		ConsecutiveSuccesses: 0,
		LastCycleAt:          time.Date(2026, 2, 27, 18, 0, 0, 0, time.UTC),
	}

	if hasStateChanged(base, base) {
		t.Fatalf("hasStateChanged(base, base) = true, want false")
	}

	statusChanged := base
	statusChanged.Status = state.StatusFailing
	if !hasStateChanged(base, statusChanged) {
		t.Fatalf("status change was not detected")
	}

	failureCountChanged := base
	failureCountChanged.ConsecutiveFailures = 2
	if !hasStateChanged(base, failureCountChanged) {
		t.Fatalf("failure counter change was not detected")
	}

	successCountChanged := base
	successCountChanged.ConsecutiveSuccesses = 1
	if !hasStateChanged(base, successCountChanged) {
		t.Fatalf("success counter change was not detected")
	}

	timestampOnlyChanged := base
	timestampOnlyChanged.LastCycleAt = base.LastCycleAt.Add(time.Second)
	if hasStateChanged(base, timestampOnlyChanged) {
		t.Fatalf("timestamp-only change should be ignored")
	}
}
