package monitor

import (
	"context"
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
		HTTP: &spec.HTTPSpec{
			Name: "s",
			Cycles: spec.SpecCycles{
				Failure: 0,
				Success: 0,
			},
		},
	}

	cycles := specCycles(parsedSpec)
	failureThreshold := thresholdOrDefault(cycles.Failure, 1)
	successThreshold := thresholdOrDefault(cycles.Success, 1)

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
		LastCycleStartedAt:   time.Date(2026, 2, 27, 17, 59, 59, 0, time.UTC),
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

	startTimestampOnlyChanged := base
	startTimestampOnlyChanged.LastCycleStartedAt = base.LastCycleStartedAt.Add(time.Second)
	if hasStateChanged(base, startTimestampOnlyChanged) {
		t.Fatalf("start timestamp-only change should be ignored")
	}
}

func TestMergedMailRecipients(t *testing.T) {
	got := mergedMailRecipients(
		[]string{"ops@example.com", "security@example.com"},
		[]string{"team@example.com", " ops@example.com ", "", "security@example.com"},
	)
	want := []string{"ops@example.com", "security@example.com", "team@example.com"}
	if len(got) != len(want) {
		t.Fatalf("len(mergedMailRecipients) = %d, want %d (%v)", len(got), len(want), got)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("mergedMailRecipients[%d] = %q, want %q", idx, got[idx], want[idx])
		}
	}
}

func TestShouldRunSpecInCycle(t *testing.T) {
	always := spec.Spec{HTTP: &spec.HTTPSpec{Name: "always"}}
	if !shouldRunSpecInCycle(always, 1) {
		t.Fatalf("spec without every_cycles should run on cycle 1")
	}

	everyThree := spec.Spec{Probe: &spec.ProbeSpec{Name: "probe", EveryCycles: 3}}
	if shouldRunSpecInCycle(everyThree, 1) {
		t.Fatalf("every_cycles=3 should not run on cycle 1")
	}
	if shouldRunSpecInCycle(everyThree, 2) {
		t.Fatalf("every_cycles=3 should not run on cycle 2")
	}
	if !shouldRunSpecInCycle(everyThree, 3) {
		t.Fatalf("every_cycles=3 should run on cycle 3")
	}
}

func TestDeterministicSpecJitter(t *testing.T) {
	maxJitter := 5 * time.Second
	specID := "http:api-health"

	first := deterministicSpecJitter(specID, maxJitter)
	second := deterministicSpecJitter(specID, maxJitter)
	if first != second {
		t.Fatalf("deterministicSpecJitter should be stable: first=%v second=%v", first, second)
	}
	if first < 0 || first >= maxJitter {
		t.Fatalf("deterministicSpecJitter out of range: got %v max %v", first, maxJitter)
	}
}

func TestSpecStartDelayOnlyOnFirstCycle(t *testing.T) {
	runner := &Runner{startupJitter: 3 * time.Second}
	parsedSpec := spec.Spec{HTTP: &spec.HTTPSpec{Name: "api-health"}}

	firstCycleDelay := runner.specStartDelay(parsedSpec, 1)
	if firstCycleDelay < 0 || firstCycleDelay >= runner.startupJitter {
		t.Fatalf("specStartDelay(first cycle) out of range: got %v", firstCycleDelay)
	}
	if got := runner.specStartDelay(parsedSpec, 2); got != 0 {
		t.Fatalf("specStartDelay(non-first cycle) = %v, want 0", got)
	}
}

func TestSleepWithContextReturnsFalseWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if ok := sleepWithContext(ctx, time.Second); ok {
		t.Fatalf("sleepWithContext canceled context = true, want false")
	}
}
