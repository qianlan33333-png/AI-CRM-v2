package migration

import "errors"

type RunMode string

const (
	ModePreflight   RunMode = "preflight"
	ModeFull        RunMode = "full"
	ModeIncremental RunMode = "incremental"
	ModeReconcile   RunMode = "reconcile"
)

var (
	ErrInvalidRunMode     = errors.New("invalid DM01 run mode")
	ErrSourcePayloadDrift = errors.New("DM01 source payload drift")
)

func (mode RunMode) Valid() bool {
	return mode == ModePreflight || mode == ModeFull || mode == ModeIncremental || mode == ModeReconcile
}
