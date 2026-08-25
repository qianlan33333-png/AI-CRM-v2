package app

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

var errPaidSettlementRowNotFound = errors.New("paid settlement row not found")

func ErrPaidSettlementRowNotFound() error { return errPaidSettlementRowNotFound }

type PaidProductSnapshot struct {
	ID, Version int64
	Kind        productport.PaidSettlementProductKind
}

type PaidEntitlementRecord struct {
	ID         productport.EntitlementID
	ProductID  productport.ID
	OrderID    int64
	CustomerID int64
	State      string
	Version    int64
	Digest     [32]byte
	GrantedAt  time.Time
	RevokedAt  *time.Time
}

type PaidMemberRecord struct {
	MemberRef  string
	ProductID  int64
	CustomerID int64
	State      string
	Version    int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type PaidSettlementStore interface {
	LockPaidProduct(context.Context, productport.ID, int64, productport.PaidSettlementProductKind) (PaidProductSnapshot, error)
	CreatePaidEntitlement(context.Context, productport.PaidSettlementCommand) (PaidEntitlementRecord, error)
	LockPaidEntitlement(context.Context, int64) (PaidEntitlementRecord, error)
	RevokePaidEntitlement(context.Context, int64, int64, [32]byte, time.Time) (PaidEntitlementRecord, error)
	CreatePaidMember(context.Context, string, productport.PaidSettlementCommand, productport.EntitlementID) (PaidMemberRecord, error)
	LockPaidMember(context.Context, int64) (PaidMemberRecord, error)
	RemovePaidMember(context.Context, int64, int64, [32]byte, time.Time) (PaidMemberRecord, error)
}

type PaidSettlementService struct {
	store  PaidSettlementStore
	events eventport.Appender
}

var _ productport.PaidSettlementWriter = (*PaidSettlementService)(nil)

func NewPaidSettlementService(store PaidSettlementStore, events eventport.Appender) (*PaidSettlementService, error) {
	if store == nil || events == nil {
		return nil, productport.ErrPaidSettlementUnavailable
	}
	return &PaidSettlementService{store: store, events: events}, nil
}

func (service *PaidSettlementService) ApplyPaidSettlement(ctx context.Context, command productport.PaidSettlementCommand) (productport.PaidSettlementResult, error) {
	if service == nil || service.store == nil || service.events == nil || !validPaidSettlement(command) {
		return productport.PaidSettlementResult{}, productport.ErrPaidSettlementInvalid
	}
	product, err := service.store.LockPaidProduct(ctx, command.ProductID, command.ProductVersion, command.ProductKind)
	if err != nil || product.ID != int64(command.ProductID) || product.Version != command.ProductVersion || product.Kind != command.ProductKind {
		return productport.PaidSettlementResult{}, classifyPaidSettlement(err)
	}
	if command.Action == productport.PaidSettlementGrant {
		return service.grant(ctx, command)
	}
	return service.compensate(ctx, command)
}

func (service *PaidSettlementService) grant(ctx context.Context, command productport.PaidSettlementCommand) (productport.PaidSettlementResult, error) {
	if existing, err := service.store.LockPaidEntitlement(ctx, command.OrderID); err == nil {
		if !samePaidEntitlement(existing, command) || existing.State != "active" {
			return productport.PaidSettlementResult{}, productport.ErrPaidSettlementConflict
		}
		return service.existingResult(ctx, command, existing)
	} else if !errors.Is(err, errPaidSettlementRowNotFound) {
		return productport.PaidSettlementResult{}, classifyPaidSettlement(err)
	}

	entitlement, err := service.store.CreatePaidEntitlement(ctx, command)
	if err != nil || !samePaidEntitlement(entitlement, command) || entitlement.State != "active" || entitlement.Version != 1 {
		return productport.PaidSettlementResult{}, classifyPaidSettlement(err)
	}
	result := productport.PaidSettlementResult{EntitlementID: entitlement.ID, State: entitlement.State, Version: entitlement.Version}
	if command.ProductKind == productport.PaidSettlementServicePeriod {
		member, memberErr := service.store.CreatePaidMember(ctx, paidMemberRef(command.SettlementReceiptDigest), command, entitlement.ID)
		if memberErr != nil || member.State != "active" || member.ProductID != int64(command.ProductID) || member.CustomerID != command.CustomerID {
			return productport.PaidSettlementResult{}, classifyPaidSettlement(memberErr)
		}
		result.MemberRef = member.MemberRef
	}
	if err = service.append(ctx, eventport.EvProductEntitlementGranted, command, entitlement.ID, result.MemberRef); err != nil {
		return productport.PaidSettlementResult{}, err
	}
	if result.MemberRef != "" {
		if err = service.append(ctx, eventport.EvServicePeriodMemberChanged, command, entitlement.ID, result.MemberRef); err != nil {
			return productport.PaidSettlementResult{}, err
		}
	}
	return result, nil
}

func (service *PaidSettlementService) existingResult(ctx context.Context, command productport.PaidSettlementCommand, entitlement PaidEntitlementRecord) (productport.PaidSettlementResult, error) {
	result := productport.PaidSettlementResult{EntitlementID: entitlement.ID, State: entitlement.State, Version: entitlement.Version}
	if command.ProductKind != productport.PaidSettlementServicePeriod {
		return result, nil
	}
	member, err := service.store.LockPaidMember(ctx, command.OrderID)
	if err != nil || member.State != "active" || member.ProductID != int64(command.ProductID) || member.CustomerID != command.CustomerID {
		return productport.PaidSettlementResult{}, classifyPaidSettlement(err)
	}
	result.MemberRef = member.MemberRef
	return result, nil
}

func (service *PaidSettlementService) compensate(ctx context.Context, command productport.PaidSettlementCommand) (productport.PaidSettlementResult, error) {
	entitlement, err := service.store.LockPaidEntitlement(ctx, command.OrderID)
	if err != nil || !samePaidEntitlement(entitlement, command) {
		return productport.PaidSettlementResult{}, classifyPaidSettlement(err)
	}
	result := productport.PaidSettlementResult{EntitlementID: entitlement.ID, State: entitlement.State, Version: entitlement.Version}
	if entitlement.State == "active" {
		entitlement, err = service.store.RevokePaidEntitlement(ctx, command.OrderID, entitlement.Version, command.SettlementReceiptDigest, command.OccurredAt)
		if err != nil || entitlement.State != "revoked" {
			return productport.PaidSettlementResult{}, classifyPaidSettlement(err)
		}
		result.State, result.Version = entitlement.State, entitlement.Version
		if err = service.append(ctx, eventport.EvProductEntitlementRevoked, command, entitlement.ID, ""); err != nil {
			return productport.PaidSettlementResult{}, err
		}
	}
	if command.ProductKind == productport.PaidSettlementServicePeriod {
		member, memberErr := service.store.LockPaidMember(ctx, command.OrderID)
		if memberErr != nil {
			return productport.PaidSettlementResult{}, classifyPaidSettlement(memberErr)
		}
		result.MemberRef = member.MemberRef
		if member.State != "removed" {
			member, memberErr = service.store.RemovePaidMember(ctx, command.OrderID, member.Version, command.SettlementReceiptDigest, command.OccurredAt)
			if memberErr != nil || member.State != "removed" {
				return productport.PaidSettlementResult{}, classifyPaidSettlement(memberErr)
			}
			if err = service.append(ctx, eventport.EvServicePeriodMemberChanged, command, entitlement.ID, member.MemberRef); err != nil {
				return productport.PaidSettlementResult{}, err
			}
		}
	}
	return result, nil
}

func (service *PaidSettlementService) append(ctx context.Context, eventType string, command productport.PaidSettlementCommand, entitlementID productport.EntitlementID, memberRef string) error {
	payload, err := json.Marshal(struct {
		OrderID       int64                            `json:"order_id"`
		ProductID     productport.ID                   `json:"product_id"`
		EntitlementID productport.EntitlementID        `json:"entitlement_id"`
		MemberRef     string                           `json:"member_ref,omitempty"`
		Action        productport.PaidSettlementAction `json:"action"`
	}{command.OrderID, command.ProductID, entitlementID, memberRef, command.Action})
	if err != nil {
		return productport.ErrPaidSettlementUnavailable
	}
	key := fmt.Sprintf("pe01.product.%s:%s:%s", command.Action, hex.EncodeToString(command.SettlementReceiptDigest[:]), eventType)
	_, err = service.events.Append(ctx, eventport.Event{Type: eventType, CustomerID: eventport.CustomerID(command.CustomerID), Payload: payload, OccurredAt: command.OccurredAt, IdempotencyKey: key})
	return err
}

func validPaidSettlement(command productport.PaidSettlementCommand) bool {
	if command.ProductID < 1 || command.ProductVersion < 1 || command.OrderID < 1 || command.CustomerID < 1 || command.OccurredAt.IsZero() || allZero(command.SettlementReceiptDigest) {
		return false
	}
	if command.Action != productport.PaidSettlementGrant && command.Action != productport.PaidSettlementCompensate {
		return false
	}
	return command.ProductKind == productport.PaidSettlementOrdinary || command.ProductKind == productport.PaidSettlementServicePeriod
}

func samePaidEntitlement(record PaidEntitlementRecord, command productport.PaidSettlementCommand) bool {
	return record.ID > 0 && record.ProductID == command.ProductID && record.OrderID == command.OrderID && record.CustomerID == command.CustomerID && record.Version > 0 && subtle.ConstantTimeCompare(record.Digest[:], command.SettlementReceiptDigest[:]) == 1
}

func paidMemberRef(digest [32]byte) string {
	return "spm_" + base64.RawURLEncoding.EncodeToString(digest[:16])
}

func allZero(digest [32]byte) bool {
	var zero [32]byte
	return subtle.ConstantTimeCompare(digest[:], zero[:]) == 1
}

func classifyPaidSettlement(err error) error {
	switch {
	case errors.Is(err, errPaidSettlementRowNotFound):
		return productport.ErrPaidSettlementConflict
	case errors.Is(err, productport.ErrPaidSettlementConflict):
		return err
	default:
		return errors.Join(productport.ErrPaidSettlementUnavailable, err)
	}
}
