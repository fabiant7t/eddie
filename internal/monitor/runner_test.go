package monitor

import (
	"testing"

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
