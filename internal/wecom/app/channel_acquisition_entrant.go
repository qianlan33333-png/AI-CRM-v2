package app

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

var (
	ErrInvalidChannelAcquisitionEntrant = errors.New("invalid channel acquisition entrant")
	ErrChannelAcquisitionEntrantFailed  = errors.New("channel acquisition entrant failed")
)

// ChannelAcquisitionEntrantInput is assembled from the durable typed callback
// fact. The raw ExternalUserID remains inside this WeCom/Identity boundary;
// it never crosses into the Contact receipt command.
type ChannelAcquisitionEntrantInput struct {
	InboxID   int64
	SourceKey string
	Fact      ExternalContactCallbackFact
}

// ChannelAcquisitionEntrantResult tells the inbox worker whether it must use
// the existing Identity pending flow after the Contact receipt has been
// persisted. Pending does not mean a Customer was created.
type ChannelAcquisitionEntrantResult struct {
	Receipt                 contactport.ChannelAcquisitionEntrantReceipt
	IdentityPendingRequired bool
}

type channelAcquisitionEntrantUnitOfWork interface {
	Within(context.Context, func(context.Context) error) error
}

// ChannelAcquisitionEntrantService coordinates the narrow CH03 decision. It
// uses the frozen CH02 CorpID+State resolver and delegates all Contact-owned
// receipt/event writes to a transaction-bound Contact port.
type ChannelAcquisitionEntrantService struct {
	uow         channelAcquisitionEntrantUnitOfWork
	correlation contactport.AcquisitionAssetCorrelationResolver
	identities  identityport.AcquisitionEntrantIdentityResolver
	receipts    contactport.ChannelAcquisitionEntrantRecorder
}

func NewChannelAcquisitionEntrantService(
	uow channelAcquisitionEntrantUnitOfWork,
	correlation contactport.AcquisitionAssetCorrelationResolver,
	identities identityport.AcquisitionEntrantIdentityResolver,
	receipts contactport.ChannelAcquisitionEntrantRecorder,
) (*ChannelAcquisitionEntrantService, error) {
	if isNilDependency(uow) || isNilDependency(correlation) || isNilDependency(identities) || isNilDependency(receipts) {
		return nil, ErrInvalidChannelAcquisitionEntrant
	}
	return &ChannelAcquisitionEntrantService{uow: uow, correlation: correlation, identities: identities, receipts: receipts}, nil
}

// Process never selects a latest/current asset. An unresolved or ambiguous
// state is persisted as such and never reaches the identity/customer path.
func (service *ChannelAcquisitionEntrantService) Process(ctx context.Context, input ChannelAcquisitionEntrantInput) (ChannelAcquisitionEntrantResult, error) {
	if service == nil || ctx == nil || isNilDependency(service.uow) || isNilDependency(service.correlation) ||
		isNilDependency(service.identities) || isNilDependency(service.receipts) || !validChannelAcquisitionEntrantInput(input) {
		return ChannelAcquisitionEntrantResult{}, ErrInvalidChannelAcquisitionEntrant
	}

	var result ChannelAcquisitionEntrantResult
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		if !input.Fact.IsEntrant() {
			return service.record(txCtx, input, contactport.ChannelAcquisitionEntrantIgnored, contactport.AcquisitionAssetCorrelationMatch{}, 0, &result)
		}
		if input.Fact.State == "" {
			return service.record(txCtx, input, contactport.ChannelAcquisitionEntrantUnmatchedAsset, contactport.AcquisitionAssetCorrelationMatch{}, 0, &result)
		}
		correlation, err := service.correlation.ResolveAcquisitionAssetCorrelation(txCtx, input.Fact.CorpID, input.Fact.State, input.Fact.OccurredAt)
		if err != nil {
			return errors.Join(ErrChannelAcquisitionEntrantFailed, err)
		}
		switch correlation.Cardinality {
		case contactport.AcquisitionAssetCorrelationZero:
			return service.record(txCtx, input, contactport.ChannelAcquisitionEntrantUnmatchedAsset, contactport.AcquisitionAssetCorrelationMatch{}, 0, &result)
		case contactport.AcquisitionAssetCorrelationMultiple:
			return service.record(txCtx, input, contactport.ChannelAcquisitionEntrantAmbiguousAsset, contactport.AcquisitionAssetCorrelationMatch{}, 0, &result)
		case contactport.AcquisitionAssetCorrelationOne:
			if !validCorrelationMatch(correlation.Match) {
				return ErrChannelAcquisitionEntrantFailed
			}
		default:
			return ErrChannelAcquisitionEntrantFailed
		}

		identity, err := service.identities.ResolveAcquisitionEntrantIdentity(txCtx, identityport.IDRef{
			Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:" + input.Fact.CorpID,
			Value: input.Fact.ExternalUserID, Assurance: identityport.AssuranceVerified, Source: "wecom.callback",
		})
		if err != nil {
			return errors.Join(ErrChannelAcquisitionEntrantFailed, err)
		}
		switch identity.Status {
		case identityport.AcquisitionEntrantIdentityFound:
			if identity.CustomerID < 1 {
				return ErrChannelAcquisitionEntrantFailed
			}
			return service.record(txCtx, input, contactport.ChannelAcquisitionEntrantAttributed, correlation.Match, identity.CustomerID, &result)
		case identityport.AcquisitionEntrantIdentityNotFound:
			if identity.CustomerID != 0 {
				return ErrChannelAcquisitionEntrantFailed
			}
			if err = service.record(txCtx, input, contactport.ChannelAcquisitionEntrantPendingIdentity, correlation.Match, 0, &result); err == nil {
				result.IdentityPendingRequired = true
			}
			return err
		case identityport.AcquisitionEntrantIdentityConflict:
			if identity.CustomerID != 0 {
				return ErrChannelAcquisitionEntrantFailed
			}
			return service.record(txCtx, input, contactport.ChannelAcquisitionEntrantConflict, correlation.Match, 0, &result)
		default:
			return ErrChannelAcquisitionEntrantFailed
		}
	})
	if err != nil {
		return ChannelAcquisitionEntrantResult{}, err
	}
	return result, nil
}

func (service *ChannelAcquisitionEntrantService) record(
	ctx context.Context,
	input ChannelAcquisitionEntrantInput,
	status contactport.ChannelAcquisitionEntrantStatus,
	match contactport.AcquisitionAssetCorrelationMatch,
	customerID contactport.CustomerID,
	result *ChannelAcquisitionEntrantResult,
) error {
	receipt, err := service.receipts.RecordChannelAcquisitionEntrant(ctx, contactport.ChannelAcquisitionEntrantCommand{
		InboxID: input.InboxID, SourceKey: input.SourceKey, CorpID: input.Fact.CorpID, CallbackState: input.Fact.State,
		WeComUserID: input.Fact.UserID, OccurredAt: input.Fact.OccurredAt, Status: status, Match: match, CustomerID: customerID,
	})
	if err != nil {
		return errors.Join(ErrChannelAcquisitionEntrantFailed, err)
	}
	if !validChannelAcquisitionEntrantReceipt(receipt, input.InboxID, status, match, customerID) {
		return ErrChannelAcquisitionEntrantFailed
	}
	result.Receipt = receipt
	return nil
}

func validChannelAcquisitionEntrantInput(input ChannelAcquisitionEntrantInput) bool {
	if input.InboxID < 1 || !validEntrantText(input.SourceKey, 512) || !validEntrantText(input.Fact.CorpID, 128) ||
		!validEntrantText(input.Fact.ExternalUserID, 1024) || input.Fact.OccurredAt.IsZero() {
		return false
	}
	if !input.Fact.IsEntrant() {
		return true
	}
	return validEntrantText(input.Fact.UserID, 1024) && (input.Fact.State == "" || validEntrantText(input.Fact.State, 512))
}

func validEntrantText(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && strings.TrimSpace(value) == value && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func validCorrelationMatch(match contactport.AcquisitionAssetCorrelationMatch) bool {
	return match.EffectID != "" && match.ChannelID > 0 && match.AssetVersion > 0 &&
		(match.Kind == contactport.AcquisitionAssetQRCode || match.Kind == contactport.AcquisitionAssetLink)
}

func validChannelAcquisitionEntrantReceipt(
	receipt contactport.ChannelAcquisitionEntrantReceipt,
	inboxID int64,
	wanted contactport.ChannelAcquisitionEntrantStatus,
	match contactport.AcquisitionAssetCorrelationMatch,
	customerID contactport.CustomerID,
) bool {
	if receipt.ID < 1 || receipt.InboxID != inboxID || !receipt.Status.Valid() || receipt.OccurredAt.IsZero() {
		return false
	}
	// A manual reconciliation can be the durable outcome returned by a replay.
	if receipt.Status != wanted && receipt.Status != contactport.ChannelAcquisitionEntrantReconciled {
		return false
	}
	if !validCorrelationMatch(match) {
		return receipt.EffectID == "" && receipt.ChannelID == 0 && receipt.AssetVersion == 0 && receipt.Kind == "" && receipt.CustomerID == 0 && receipt.CustomerEventID == 0
	}
	if receipt.EffectID != match.EffectID || receipt.ChannelID != match.ChannelID || receipt.Kind != match.Kind || receipt.AssetVersion != match.AssetVersion {
		return false
	}
	if wanted == contactport.ChannelAcquisitionEntrantAttributed {
		return customerID > 0 && receipt.CustomerID == customerID && receipt.CustomerEventID > 0
	}
	return customerID == 0 && receipt.CustomerID == 0 && receipt.CustomerEventID == 0
}
