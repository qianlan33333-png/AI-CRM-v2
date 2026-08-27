package membergrid

import (
	"context"
	"errors"
	"strings"
)

const (
	externalShareTokenPrefix = "mgshare1"
	minimumExternalShareID   = 16
	maximumExternalShareID   = 128
)

var (
	ErrInvalidExternalShareInput = errors.New("invalid member grid external share input")
	ErrInvalidExternalShareToken = errors.New("invalid member grid external share token")
)

// ExternalShare is the durable, non-secret share state. ShareID is an opaque
// lookup identifier, not a bearer token; the public token is derived by the
// codec and is never part of this state or a generic operation receipt.
type ExternalShare struct {
	ServiceProductID int64
	ShareID          string
	Enabled          bool
	Version          int64
}

// SetExternalShareCommand is the administration write intent. Persistence is
// responsible for reserving the IdempotencyKey with this command's payload in
// the same transaction as its version-guarded state update.
type SetExternalShareCommand struct {
	ServiceProductID int64
	Enabled          bool
	ExpectedVersion  int64
	ActorID          int64
	IdempotencyKey   string
}

// SetExternalShareRecord is the persistence seam. A store must enforce the
// ExpectedVersion compare-and-set at write time, not rely on the service's
// preceding read.
type SetExternalShareRecord struct {
	ServiceProductID int64
	Enabled          bool
	ShareID          string
	ExpectedVersion  int64
	ActorID          int64
	IdempotencyKey   string
}

// SetExternalShareResult returns a raw token only for a newly enabled share.
// Callers must deliver it directly in a URL fragment and must not put it in a
// generic idempotency receipt or subsequent settings read.
type SetExternalShareResult struct {
	Share       ExternalShare
	PublicToken string
	TokenIssued bool
}

// ExternalShareIDFactory supplies a fresh opaque identifier for every
// disabled-to-enabled transition. The domain deliberately does not choose a
// UUID implementation or persist identifiers itself.
type ExternalShareIDFactory interface {
	NewExternalShareID(context.Context) (string, error)
}

// ExternalShareStore is implemented by the later SQL/UoW integration. A
// disabled product without a stored share is represented as version zero.
// LookupEnabledExternalShare must only return a currently enabled row whose
// ShareID exactly matches the supplied value.
type ExternalShareStore interface {
	CurrentExternalShare(context.Context, int64) (ExternalShare, error)
	SetExternalShare(context.Context, SetExternalShareRecord) (ExternalShare, error)
	LookupEnabledExternalShare(context.Context, string) (ExternalShare, error)
}

// ExternalShareService contains no HTTP or SQL concerns. Its enable transition
// issues a new opaque share ID; disable clears it. Therefore a previously
// issued token cannot become valid again after disable then re-enable.
type ExternalShareService struct {
	store  ExternalShareStore
	ids    ExternalShareIDFactory
	tokens *ExternalShareTokenCodec
}

func NewExternalShareService(store ExternalShareStore, ids ExternalShareIDFactory, tokens *ExternalShareTokenCodec) (*ExternalShareService, error) {
	if nilDependency(store) || nilDependency(ids) || tokens == nil {
		return nil, errors.New("member grid external share dependencies are required")
	}
	return &ExternalShareService{store: store, ids: ids, tokens: tokens}, nil
}

func (service *ExternalShareService) SetExternalShare(ctx context.Context, command SetExternalShareCommand) (SetExternalShareResult, error) {
	if service == nil || nilDependency(service.store) || nilDependency(service.ids) || service.tokens == nil || ctx == nil || !validSetExternalShareCommand(command) {
		return SetExternalShareResult{}, ErrInvalidExternalShareInput
	}
	if err := ctx.Err(); err != nil {
		return SetExternalShareResult{}, errors.Join(ErrUnavailable, err)
	}
	current, err := service.store.CurrentExternalShare(ctx, command.ServiceProductID)
	if err != nil {
		return SetExternalShareResult{}, err
	}
	if !validExternalShare(current) || current.ServiceProductID != command.ServiceProductID {
		return SetExternalShareResult{}, ErrUnavailable
	}
	if current.Version != command.ExpectedVersion {
		return SetExternalShareResult{}, ErrConflict
	}
	if current.Enabled == command.Enabled {
		return SetExternalShareResult{Share: cloneExternalShare(current)}, nil
	}

	record := SetExternalShareRecord{
		ServiceProductID: command.ServiceProductID,
		Enabled:          command.Enabled,
		ExpectedVersion:  command.ExpectedVersion,
		ActorID:          command.ActorID,
		IdempotencyKey:   command.IdempotencyKey,
	}
	var token string
	if command.Enabled {
		record.ShareID, err = service.ids.NewExternalShareID(ctx)
		if err != nil {
			return SetExternalShareResult{}, err
		}
		if !validExternalShareID(record.ShareID) {
			return SetExternalShareResult{}, ErrUnavailable
		}
		token, err = service.tokens.Issue(record.ShareID)
		if err != nil {
			return SetExternalShareResult{}, err
		}
	}
	next, err := service.store.SetExternalShare(ctx, record)
	if err != nil {
		return SetExternalShareResult{}, err
	}
	if !validExternalShare(next) || next.ServiceProductID != command.ServiceProductID || next.Enabled != command.Enabled || next.Version != current.Version+1 || next.ShareID != record.ShareID {
		return SetExternalShareResult{}, ErrUnavailable
	}
	return SetExternalShareResult{Share: cloneExternalShare(next), PublicToken: token, TokenIssued: command.Enabled}, nil
}

// ResolvePublicExternalShare checks token authenticity and the store's live
// enabled state. It deliberately does not issue a token or read member data.
func (service *ExternalShareService) ResolvePublicExternalShare(ctx context.Context, token string) (ExternalShare, error) {
	if service == nil || nilDependency(service.store) || service.tokens == nil || ctx == nil {
		return ExternalShare{}, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return ExternalShare{}, errors.Join(ErrUnavailable, err)
	}
	shareID, err := service.tokens.Verify(token)
	if err != nil {
		return ExternalShare{}, err
	}
	share, err := service.store.LookupEnabledExternalShare(ctx, shareID)
	if err != nil {
		return ExternalShare{}, err
	}
	if !validExternalShare(share) || !share.Enabled || share.ShareID != shareID {
		return ExternalShare{}, ErrUnavailable
	}
	return cloneExternalShare(share), nil
}

func validSetExternalShareCommand(command SetExternalShareCommand) bool {
	return command.ServiceProductID > 0 && command.ExpectedVersion >= 0 && command.ActorID > 0 && validIdempotencyKey(command.IdempotencyKey)
}

func validExternalShare(value ExternalShare) bool {
	if value.ServiceProductID < 1 || value.Version < 0 {
		return false
	}
	return !value.Enabled && value.ShareID == "" || value.Enabled && value.Version > 0 && validExternalShareID(value.ShareID)
}

func validExternalShareID(value string) bool {
	if len(value) < minimumExternalShareID || len(value) > maximumExternalShareID || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_') {
			return false
		}
	}
	return true
}

func cloneExternalShare(value ExternalShare) ExternalShare { return value }
