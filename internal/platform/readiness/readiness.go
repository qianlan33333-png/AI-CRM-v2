// Package readiness evaluates the public system-health contract from safe,
// already-read observations. It performs no network or database I/O.
package readiness

const (
	// MaxUnknownAfterDispatchCount keeps the public queue observation
	// low-cardinality. It never exposes individual dispatches.
	MaxUnknownAfterDispatchCount uint16 = 99
)

// Component is a fixed system-health component name.
type Component string

const (
	ComponentWeCom       Component = "wecom"
	ComponentRelease     Component = "release"
	ComponentRuntimeUnit Component = "runtime_units"
	ComponentDatabase    Component = "database"
	ComponentMigration   Component = "migration"
	ComponentQueues      Component = "queues"
)

var orderedComponents = [...]Component{
	ComponentWeCom,
	ComponentRelease,
	ComponentRuntimeUnit,
	ComponentDatabase,
	ComponentMigration,
	ComponentQueues,
}

// ComponentStatus is the safe, public state of one fixed component.
type ComponentStatus string

const (
	ComponentReady   ComponentStatus = "ready"
	ComponentWarning ComponentStatus = "warning"
	ComponentFailed  ComponentStatus = "failed"
)

// DatabaseKind identifies the configured database without disclosing a URL or
// other connection information.
type DatabaseKind string

const (
	DatabasePostgres DatabaseKind = "postgres"
	DatabaseFixture  DatabaseKind = "fixture"
	DatabaseMissing  DatabaseKind = "missing"
)

// ProbeStatus reports the result of a completed, legal read-only probe.
type ProbeStatus string

const (
	ProbeHealthy ProbeStatus = "healthy"
	ProbeFailed  ProbeStatus = "failed"
)

// MigrationCompatibility is the migration state reduced to its health effect.
type MigrationCompatibility string

const (
	MigrationCompatible      MigrationCompatibility = "compatible"
	MigrationCompatibleAhead MigrationCompatibility = "compatible_ahead"
	MigrationIncompatible    MigrationCompatibility = "incompatible"
)

// Input contains only safe observations gathered through owner-approved
// read-only ports by the eventual transport integration.
type Input struct {
	Production   bool
	Database     DatabaseObservation
	WeCom        WeComObservation
	Release      ReleaseObservation
	RuntimeUnits ComponentObservation
	Migration    MigrationObservation
	Queues       QueueObservation
}

// DatabaseObservation is the safe result of a database readiness probe.
type DatabaseObservation struct {
	Kind  DatabaseKind
	Probe ProbeStatus
}

// WeComObservation is configuration and conflict state only. Evaluating it
// never performs a WeCom call.
type WeComObservation struct {
	Conflict         bool
	RealCallsEnabled bool
}

// ReleaseObservation contains no release identifier; only completeness is
// needed for readiness.
type ReleaseObservation struct {
	SHAComplete bool
}

// ComponentObservation represents a safe local health observation.
type ComponentObservation struct {
	Status ComponentStatus
}

// MigrationObservation is the safe compatibility result from a legal
// read-only migration source.
type MigrationObservation struct {
	Compatibility MigrationCompatibility
}

// QueueObservation contains only bounded aggregate values.
type QueueObservation struct {
	Probe                ProbeStatus
	BudgetExhausted      bool
	UnknownAfterDispatch uint64
}

// ComponentReport is the safe public report for one component. Optional
// fields occur only on their matching component.
type ComponentReport struct {
	Name                      Component       `json:"name"`
	Status                    ComponentStatus `json:"status"`
	RealCallsEnabled          *bool           `json:"real_calls_enabled,omitempty"`
	UnknownAfterDispatchCount *uint16         `json:"unknown_after_dispatch_count,omitempty"`
}

// Response is the fixed public response for LEGACY-API-0741.
type Response struct {
	OK                bool              `json:"ok"`
	Status            string            `json:"status"`
	HTTPStatus        int               `json:"http_status"`
	FailedComponents  []Component       `json:"failed_components"`
	WarningComponents []Component       `json:"warning_components"`
	Components        []ComponentReport `json:"components"`
	PIIInOutput       bool              `json:"pii_in_output"`
	SecretsInOutput   bool              `json:"secrets_in_output"`
}

// Evaluate applies the health contract to safe observations. Unknown input is
// fail-closed; no raw probe or dependency data is placed in Response.
func Evaluate(input Input) Response {
	states := map[Component]ComponentStatus{
		ComponentWeCom:       wecomStatus(input.WeCom),
		ComponentRelease:     releaseStatus(input.Production, input.Release),
		ComponentRuntimeUnit: safeComponentStatus(input.RuntimeUnits.Status),
		ComponentDatabase:    databaseStatus(input.Production, input.Database),
		ComponentMigration:   migrationStatus(input.Migration),
		ComponentQueues:      queuesStatus(input.Queues),
	}

	realCallsEnabled := input.WeCom.RealCallsEnabled
	unknownAfterDispatch := boundedUnknownAfterDispatch(input.Queues.UnknownAfterDispatch)
	response := Response{
		FailedComponents:  make([]Component, 0),
		WarningComponents: make([]Component, 0),
		Components:        make([]ComponentReport, 0, len(orderedComponents)),
		PIIInOutput:       false,
		SecretsInOutput:   false,
	}

	for _, component := range orderedComponents {
		report := ComponentReport{Name: component, Status: states[component]}
		switch component {
		case ComponentWeCom:
			report.RealCallsEnabled = &realCallsEnabled
		case ComponentQueues:
			report.UnknownAfterDispatchCount = &unknownAfterDispatch
		}
		response.Components = append(response.Components, report)

		switch report.Status {
		case ComponentFailed:
			response.FailedComponents = append(response.FailedComponents, component)
		case ComponentWarning:
			response.WarningComponents = append(response.WarningComponents, component)
		}
	}

	response.OK = len(response.FailedComponents) == 0
	if response.OK {
		response.Status = "ready"
		response.HTTPStatus = 200
	} else {
		response.Status = "not_ready"
		response.HTTPStatus = 503
	}
	return response
}

func wecomStatus(observation WeComObservation) ComponentStatus {
	if observation.Conflict {
		return ComponentFailed
	}
	return ComponentReady
}

func releaseStatus(production bool, observation ReleaseObservation) ComponentStatus {
	if !observation.SHAComplete {
		if production {
			return ComponentFailed
		}
		return ComponentWarning
	}
	return ComponentReady
}

func databaseStatus(production bool, observation DatabaseObservation) ComponentStatus {
	if observation.Probe != ProbeHealthy {
		return ComponentFailed
	}
	if observation.Kind != DatabasePostgres && observation.Kind != DatabaseFixture {
		return ComponentFailed
	}
	if production && observation.Kind != DatabasePostgres {
		return ComponentFailed
	}
	return ComponentReady
}

func migrationStatus(observation MigrationObservation) ComponentStatus {
	switch observation.Compatibility {
	case MigrationCompatible:
		return ComponentReady
	case MigrationCompatibleAhead:
		return ComponentWarning
	default:
		return ComponentFailed
	}
}

func queuesStatus(observation QueueObservation) ComponentStatus {
	if observation.BudgetExhausted {
		return ComponentWarning
	}
	if observation.Probe != ProbeHealthy {
		return ComponentFailed
	}
	return ComponentReady
}

func safeComponentStatus(status ComponentStatus) ComponentStatus {
	switch status {
	case ComponentReady, ComponentWarning, ComponentFailed:
		return status
	default:
		return ComponentFailed
	}
}

func boundedUnknownAfterDispatch(value uint64) uint16 {
	if value > uint64(MaxUnknownAfterDispatchCount) {
		return MaxUnknownAfterDispatchCount
	}
	return uint16(value)
}
