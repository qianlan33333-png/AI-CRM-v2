package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

type IdentityContactFactSource string

const (
	IdentityContactFromCallback IdentityContactFactSource = "callback_inbox"
	IdentityContactFromSync     IdentityContactFactSource = "directory_sync"
)

var (
	ErrInvalidIdentityContactProcessor = errors.New("invalid WeCom identity/contact processor")
	ErrInvalidIdentityContactFact      = errors.New("invalid WeCom identity/contact fact")
	ErrIdentityContactIngestFailed     = errors.New("WeCom identity/contact ingest failed")
	ErrInvalidIdentityContactResult    = errors.New("invalid WeCom identity/contact result")
)

// IdentityContactFact is the minimum verified business fact W5 accepts from
// either the durable callback inbox or W4 directory sync. FactID is an opaque,
// stable source identifier; it is hashed with CorpID before crossing into a
// receipt key so identical provider IDs in different enterprises cannot clash.
type IdentityContactFact struct {
	Source         IdentityContactFactSource
	FactID         string
	CorpID         string
	ExternalUserID string
	OccurredAt     time.Time
}

// IdentityIngestor is deliberately the existing Identity Ingest port method.
// Identity owns Resolve/Bind and the transaction-bound Contact create/timeline
// orchestration; WeCom must never reproduce those writes directly.
type IdentityIngestor interface {
	Ingest(context.Context, identityport.IngestCommand) (identityport.IngestResult, error)
}

type IdentityContactProcessor struct {
	identity IdentityIngestor
}

func NewIdentityContactProcessor(identity IdentityIngestor) (*IdentityContactProcessor, error) {
	if isNilDependency(identity) {
		return nil, ErrInvalidIdentityContactProcessor
	}
	return &IdentityContactProcessor{identity: identity}, nil
}

// Process delegates one verified WeCom identity fact to Identity. Replays use
// the same command key; an interruption is returned to the job runner so it can
// retry, while attributed/pending/conflict are all durable terminal outcomes.
func (processor *IdentityContactProcessor) Process(ctx context.Context, fact IdentityContactFact) (identityport.IngestResult, error) {
	if processor == nil || ctx == nil || isNilDependency(processor.identity) || !validIdentityContactFact(fact) {
		return identityport.IngestResult{}, ErrInvalidIdentityContactFact
	}

	eventType, provenance, payload := identityContactSourceContract(fact.Source)
	digest := sha256.Sum256([]byte("wecom.identity-contact.v1\x00" + string(fact.Source) + "\x00" + fact.CorpID + "\x00" + fact.FactID))
	command := identityport.IngestCommand{
		Refs: []identityport.IDRef{{
			Kind:      identityport.KindWeComExternalUserID,
			Scope:     "wecom-corp:" + fact.CorpID,
			Value:     fact.ExternalUserID,
			Assurance: identityport.AssuranceVerified,
			Source:    provenance,
		}},
		EventType:      eventType,
		Payload:        append(json.RawMessage(nil), payload...),
		Source:         provenance,
		OccurredAt:     fact.OccurredAt.UTC(),
		IdempotencyKey: "wecom.identity_contact:" + string(fact.Source) + ":" + hex.EncodeToString(digest[:]),
	}
	result, err := processor.identity.Ingest(ctx, command)
	if err != nil {
		return identityport.IngestResult{}, fmt.Errorf("%w: %w", ErrIdentityContactIngestFailed, err)
	}
	switch result.Status {
	case identityport.IngestAttributed, identityport.IngestPending, identityport.IngestConflict:
		return result, nil
	default:
		return identityport.IngestResult{}, ErrInvalidIdentityContactResult
	}
}

func identityContactSourceContract(source IdentityContactFactSource) (string, string, json.RawMessage) {
	switch source {
	case IdentityContactFromCallback:
		return "wecom.external_contact.callback_observed", "wecom.callback", json.RawMessage(`{"source":"callback_inbox"}`)
	case IdentityContactFromSync:
		return "wecom.external_contact.sync_observed", "wecom.sync", json.RawMessage(`{"source":"directory_sync"}`)
	default:
		return "", "", nil
	}
}

func validIdentityContactFact(fact IdentityContactFact) bool {
	if fact.Source != IdentityContactFromCallback && fact.Source != IdentityContactFromSync {
		return false
	}
	return validIdentityContactText(fact.FactID, 512, false) &&
		validIdentityContactText(fact.CorpID, 256, true) &&
		validIdentityContactText(fact.ExternalUserID, 1024, false) &&
		!fact.OccurredAt.IsZero()
}

func validIdentityContactText(value string, maximum int, rejectSpace bool) bool {
	if value == "" || len(value) > maximum || strings.TrimSpace(value) != value || !utf8.ValidString(value) ||
		strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return false
	}
	return !rejectSpace || strings.IndexFunc(value, unicode.IsSpace) < 0
}
