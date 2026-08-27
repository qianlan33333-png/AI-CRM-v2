package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	opsstore "github.com/qianlan33333-png/AI-CRM-v2/internal/ops/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/platform/readiness"
)

const (
	systemHealthPath              = "/api/system/health"
	systemReadinessPath           = "/readyz"
	systemHealthObservationBudget = 750 * time.Millisecond
)

var fullReleaseSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

type systemHealthObservationSource interface {
	Observe(context.Context) readiness.Input
}

type postgresSystemHealthSource struct {
	platformObserver interface {
		Observe(context.Context) (opsstore.ReadinessSnapshot, error)
	}
	queueObserver interface {
		CountOutcomeUnknown(context.Context) (uint64, error)
	}
	production       bool
	releaseSHA       string
	realCallsEnabled bool
}

func (source postgresSystemHealthSource) Observe(parent context.Context) readiness.Input {
	input := readiness.Input{
		Production: source.production,
		Database: readiness.DatabaseObservation{
			Kind:  readiness.DatabasePostgres,
			Probe: readiness.ProbeFailed,
		},
		WeCom: readiness.WeComObservation{RealCallsEnabled: source.realCallsEnabled},
		Release: readiness.ReleaseObservation{
			SHAComplete: fullReleaseSHA.MatchString(strings.TrimSpace(source.releaseSHA)),
		},
		RuntimeUnits: readiness.ComponentObservation{Status: readiness.ComponentWarning},
		Migration:    readiness.MigrationObservation{Compatibility: readiness.MigrationIncompatible},
		Queues:       readiness.QueueObservation{Probe: readiness.ProbeFailed},
	}
	if source.platformObserver == nil || source.queueObserver == nil {
		return input
	}
	ctx, cancel := context.WithTimeout(parent, systemHealthObservationBudget)
	defer cancel()
	snapshot, err := source.platformObserver.Observe(ctx)
	if err != nil {
		return input
	}
	input.Database.Probe = readiness.ProbeHealthy
	if snapshot.CurrentSchemaCompatible {
		input.Migration.Compatibility = readiness.MigrationCompatible
	}
	unknownAfterDispatch, err := source.queueObserver.CountOutcomeUnknown(ctx)
	if err == nil {
		input.Queues = readiness.QueueObservation{Probe: readiness.ProbeHealthy, UnknownAfterDispatch: unknownAfterDispatch}
	} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		input.Queues = readiness.QueueObservation{Probe: readiness.ProbeFailed, BudgetExhausted: true}
	}
	return input
}

type systemHealthHandler struct {
	source systemHealthObservationSource
}

func newSystemHealthHandler(source systemHealthObservationSource) (*systemHealthHandler, error) {
	if source == nil {
		return nil, errInvalidAPIComponent
	}
	return &systemHealthHandler{source: source}, nil
}

func (handler *systemHealthHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	response := readiness.Evaluate(handler.source.Observe(request.Context()))
	payload, err := json.Marshal(response)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(response.HTTPStatus)
	_, _ = writer.Write(payload)
}
