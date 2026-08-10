package scheduler

import (
	"errors"
	"reflect"
	"regexp"
	"time"

	jobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	queueriver "github.com/riverqueue/river"
)

var (
	ErrInvalidRegistry   = errors.New("invalid scheduler registry")
	ErrInvalidDefinition = errors.New("invalid periodic job definition")
	ErrDuplicateID       = errors.New("duplicate periodic job ID")
	ErrInvalidInterval   = errors.New("periodic interval must be at least one second")
)

var periodicIDPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_\-\[\]<>/.\x{00B7}:+]+$`)

// Schedule describes when a periodic job should next be enqueued. Implementations
// calculate times only; they must not start timers or goroutines.
type Schedule interface {
	Next(time.Time) time.Time
}

type Definition struct {
	ID         string
	Queue      jobqueue.Queue
	Schedule   Schedule
	Args       queueriver.JobArgs
	Options    *queueriver.InsertOpts
	RunOnStart bool
}

// Plan is the immutable result of the repository's single periodic registration pass.
type Plan struct {
	jobs []*queueriver.PeriodicJob
}

// Build is the only production entry point that converts periodic definitions into
// River registrations. It validates worker ownership and explicit queue selection
// before any River client is created.
func Build(workers *jobqueue.WorkerRegistry, definitions []Definition) (*Plan, error) {
	if workers == nil {
		return nil, ErrInvalidRegistry
	}
	jobs := make([]*queueriver.PeriodicJob, 0, len(definitions))
	ids := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		if len(definition.ID) >= 128 || !periodicIDPattern.MatchString(definition.ID) || isNil(definition.Schedule) {
			return nil, ErrInvalidDefinition
		}
		if _, exists := ids[definition.ID]; exists {
			return nil, ErrDuplicateID
		}
		explicitOptions, err := workers.ExplicitOptions(definition.Queue, definition.Args, definition.Options)
		if err != nil {
			return nil, errors.Join(ErrInvalidDefinition, err)
		}

		args := definition.Args
		options := *explicitOptions
		jobs = append(jobs, queueriver.NewPeriodicJob(
			definition.Schedule,
			func() (queueriver.JobArgs, *queueriver.InsertOpts) {
				cloned := options
				return args, &cloned
			},
			&queueriver.PeriodicJobOpts{ID: definition.ID, RunOnStart: definition.RunOnStart},
		))
		ids[definition.ID] = struct{}{}
	}
	return &Plan{jobs: jobs}, nil
}

func (plan *Plan) Jobs() []*queueriver.PeriodicJob {
	if plan == nil {
		return nil
	}
	return append([]*queueriver.PeriodicJob(nil), plan.jobs...)
}

func Every(interval time.Duration) (Schedule, error) {
	if interval < time.Second {
		return nil, ErrInvalidInterval
	}
	return intervalSchedule{interval: interval}, nil
}

func Never() Schedule {
	return neverSchedule{}
}

type intervalSchedule struct {
	interval time.Duration
}

func (schedule intervalSchedule) Next(current time.Time) time.Time {
	return current.Add(schedule.interval)
}

type neverSchedule struct{}

func (neverSchedule) Next(time.Time) time.Time {
	return time.Unix(1<<63-62135596801, 999999999)
}

func isNil(value any) bool {
	reflected := reflect.ValueOf(value)
	return !reflected.IsValid() ||
		((reflected.Kind() == reflect.Chan || reflected.Kind() == reflect.Func ||
			reflected.Kind() == reflect.Interface || reflected.Kind() == reflect.Map ||
			reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Slice) && reflected.IsNil())
}
