package readiness

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestEvaluateHealthyPostgresIsReady(t *testing.T) {
	response := Evaluate(healthyProductionInput())

	if !response.OK || response.Status != "ready" || response.HTTPStatus != 200 {
		t.Fatalf("healthy postgres response = %+v", response)
	}
	if len(response.FailedComponents) != 0 || len(response.WarningComponents) != 0 {
		t.Fatalf("healthy postgres components = %+v", response)
	}
	if response.PIIInOutput || response.SecretsInOutput {
		t.Fatalf("output safety flags = pii:%t secrets:%t", response.PIIInOutput, response.SecretsInOutput)
	}
	assertFixedComponentOrder(t, response)
	if response.Components[0].RealCallsEnabled == nil || *response.Components[0].RealCallsEnabled {
		t.Fatalf("wecom configuration observation = %+v", response.Components[0])
	}
}

func TestEvaluateExplicitFixtureWithIncompleteReleaseIsWarning(t *testing.T) {
	input := healthyProductionInput()
	input.Production = false
	input.Database.Kind = DatabaseFixture
	input.Release.SHAComplete = false

	response := Evaluate(input)
	if !response.OK || response.Status != "ready" || response.HTTPStatus != 200 {
		t.Fatalf("explicit fixture response = %+v", response)
	}
	if !reflect.DeepEqual(response.WarningComponents, []Component{ComponentRelease}) {
		t.Fatalf("warning components = %v", response.WarningComponents)
	}
	if response.Components[1].Status != ComponentWarning {
		t.Fatalf("release state = %+v", response.Components[1])
	}
}

func TestEvaluateCompatibleAheadIsWarning(t *testing.T) {
	input := healthyProductionInput()
	input.Migration.Compatibility = MigrationCompatibleAhead

	response := Evaluate(input)
	if !response.OK || response.Status != "ready" || response.HTTPStatus != 200 {
		t.Fatalf("warning-only response = %+v", response)
	}
	if !reflect.DeepEqual(response.WarningComponents, []Component{ComponentMigration}) {
		t.Fatalf("warning components = %v", response.WarningComponents)
	}
	if response.Components[4].Status != ComponentWarning {
		t.Fatalf("migration state = %+v", response.Components[4])
	}
}

func TestEvaluateQueueBudgetExhaustionIsWarning(t *testing.T) {
	input := healthyProductionInput()
	input.Queues.BudgetExhausted = true

	response := Evaluate(input)
	if !response.OK || response.Status != "ready" || response.HTTPStatus != 200 {
		t.Fatalf("warning-only response = %+v", response)
	}
	if !reflect.DeepEqual(response.WarningComponents, []Component{ComponentQueues}) {
		t.Fatalf("warning components = %v", response.WarningComponents)
	}
	if response.Components[5].Status != ComponentWarning {
		t.Fatalf("queue state = %+v", response.Components[5])
	}
}

func TestEvaluateUnknownQueueProbeFailsClosed(t *testing.T) {
	input := healthyProductionInput()
	input.Queues.Probe = ProbeFailed

	response := Evaluate(input)
	if response.OK || response.Status != "not_ready" || response.HTTPStatus != 503 {
		t.Fatalf("failed queue probe response = %+v", response)
	}
	if !reflect.DeepEqual(response.FailedComponents, []Component{ComponentQueues}) {
		t.Fatalf("failed components = %v", response.FailedComponents)
	}
}

func TestEvaluateRequiredFailuresAreNotReady(t *testing.T) {
	tests := []struct {
		name      string
		change    func(*Input)
		component Component
	}{
		{
			name: "production database is missing",
			change: func(input *Input) {
				input.Database.Kind = DatabaseMissing
			},
			component: ComponentDatabase,
		},
		{
			name: "database probe failed",
			change: func(input *Input) {
				input.Database.Probe = ProbeFailed
			},
			component: ComponentDatabase,
		},
		{
			name: "migration is incompatible",
			change: func(input *Input) {
				input.Migration.Compatibility = MigrationIncompatible
			},
			component: ComponentMigration,
		},
		{
			name: "wecom has a conflict",
			change: func(input *Input) {
				input.WeCom.Conflict = true
			},
			component: ComponentWeCom,
		},
		{
			name: "production release sha is incomplete",
			change: func(input *Input) {
				input.Release.SHAComplete = false
			},
			component: ComponentRelease,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := healthyProductionInput()
			test.change(&input)

			response := Evaluate(input)
			if response.OK || response.Status != "not_ready" || response.HTTPStatus != 503 {
				t.Fatalf("failure response = %+v", response)
			}
			if !reflect.DeepEqual(response.FailedComponents, []Component{test.component}) {
				t.Fatalf("failed components = %v, want %v", response.FailedComponents, test.component)
			}
		})
	}
}

func TestEvaluateUnknownInputFailsClosed(t *testing.T) {
	input := healthyProductionInput()
	input.RuntimeUnits.Status = ComponentStatus("unknown")
	input.Migration.Compatibility = MigrationCompatibility("unknown")

	response := Evaluate(input)
	if response.OK || response.Status != "not_ready" || response.HTTPStatus != 503 {
		t.Fatalf("unknown input response = %+v", response)
	}
	if !reflect.DeepEqual(response.FailedComponents, []Component{ComponentRuntimeUnit, ComponentMigration}) {
		t.Fatalf("failed components = %v", response.FailedComponents)
	}
}

func TestEvaluateBoundsUnknownAfterDispatchAndDoesNotLeakUnsafeFields(t *testing.T) {
	input := healthyProductionInput()
	input.Queues.UnknownAfterDispatch = uint64(MaxUnknownAfterDispatchCount) + 1

	response := Evaluate(input)
	queue := response.Components[5]
	if queue.UnknownAfterDispatchCount == nil || *queue.UnknownAfterDispatchCount != MaxUnknownAfterDispatchCount {
		t.Fatalf("bounded unknown-after-dispatch count = %+v", queue)
	}

	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	for _, forbidden := range []string{
		"event", "job", "receipt", "error", "payload", "token", "postgres://", "openid", "external_userid",
	} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("public response contains forbidden %q: %s", forbidden, encoded)
		}
	}
}

func healthyProductionInput() Input {
	return Input{
		Production: true,
		Database: DatabaseObservation{
			Kind:  DatabasePostgres,
			Probe: ProbeHealthy,
		},
		WeCom:        WeComObservation{RealCallsEnabled: false},
		Release:      ReleaseObservation{SHAComplete: true},
		RuntimeUnits: ComponentObservation{Status: ComponentReady},
		Migration:    MigrationObservation{Compatibility: MigrationCompatible},
		Queues:       QueueObservation{Probe: ProbeHealthy},
	}
}

func assertFixedComponentOrder(t *testing.T, response Response) {
	t.Helper()
	got := make([]Component, 0, len(response.Components))
	for _, component := range response.Components {
		got = append(got, component.Name)
	}
	want := []Component{
		ComponentWeCom,
		ComponentRelease,
		ComponentRuntimeUnit,
		ComponentDatabase,
		ComponentMigration,
		ComponentQueues,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("component order = %v, want %v", got, want)
	}
}
