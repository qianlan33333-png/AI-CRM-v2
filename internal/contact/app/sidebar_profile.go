package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

type SidebarProfileRecord struct {
	CustomerID, OwnerStaffID int64
	Name                     string
	Extra                    json.RawMessage
	UpdatedAt                time.Time
}

type SidebarProfileReceiptReservation struct {
	ActorScope               string
	KeyDigest, PayloadDigest [32]byte
	CreatedAt                time.Time
}

type SidebarProfileReceipt struct {
	ID                       int64
	ActorScope               string
	KeyDigest, PayloadDigest [32]byte
	State                    string
	ResultSnapshot           json.RawMessage
}

type SidebarProfileStore interface {
	ReadSidebarProfile(context.Context, int64, int64) (SidebarProfileRecord, error)
	UpdateSidebarProfile(context.Context, SidebarProfileRecord, time.Time) (SidebarProfileRecord, error)
	ReserveSidebarProfileReceipt(context.Context, SidebarProfileReceiptReservation) (SidebarProfileReceipt, bool, error)
	CompleteSidebarProfileReceipt(context.Context, int64, json.RawMessage, time.Time) (SidebarProfileReceipt, error)
}

type SidebarProfileService struct {
	uow    platformport.UnitOfWork
	store  SidebarProfileStore
	events eventport.Appender
	effect SidebarProfileEffect
	now    func() time.Time
}

type SidebarProfileEffectCommand struct {
	ReceiptID, ActorID int64
	CustomerID         contactport.CustomerID
	IdempotencyKey     string
	Profile            contactport.SidebarProfile
}

type SidebarProfileEffectAcceptance struct {
	Queued                    bool
	ProviderExecutionEligible bool
}

type SidebarProfileEffect interface {
	QueueInTransaction(context.Context, SidebarProfileEffectCommand) (SidebarProfileEffectAcceptance, error)
}

func NewSidebarProfileService(uow platformport.UnitOfWork, store SidebarProfileStore, events eventport.Appender) *SidebarProfileService {
	return &SidebarProfileService{uow: uow, store: store, events: events, now: time.Now}
}

func NewSidebarProfileServiceWithEffect(uow platformport.UnitOfWork, store SidebarProfileStore, events eventport.Appender, effect SidebarProfileEffect) *SidebarProfileService {
	return &SidebarProfileService{uow: uow, store: store, events: events, effect: effect, now: time.Now}
}

func (service *SidebarProfileService) ResolveSidebarProfile(ctx context.Context, customerID contactport.CustomerID) (contactport.SidebarProfile, error) {
	return service.readSidebarProfile(ctx, customerID, 0)
}

func (service *SidebarProfileService) ReadSidebarProfile(ctx context.Context, customerID contactport.CustomerID, ownerStaffID int64) (contactport.SidebarProfile, error) {
	if ownerStaffID < 1 {
		return contactport.SidebarProfile{}, contactport.ErrSidebarProfileInvalid
	}
	return service.readSidebarProfile(ctx, customerID, ownerStaffID)
}

func (service *SidebarProfileService) readSidebarProfile(ctx context.Context, customerID contactport.CustomerID, ownerStaffID int64) (contactport.SidebarProfile, error) {
	if !sidebarProfileReady(service) || ctx == nil || customerID < 1 || ownerStaffID < 0 {
		return contactport.SidebarProfile{}, contactport.ErrSidebarProfileInvalid
	}
	var record SidebarProfileRecord
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var readErr error
		record, readErr = service.store.ReadSidebarProfile(tx, int64(customerID), ownerStaffID)
		return readErr
	})
	if err != nil {
		return contactport.SidebarProfile{}, sidebarProfileError(err)
	}
	return sidebarProfileProjection(record)
}

func (service *SidebarProfileService) UpdateSidebarProfile(ctx context.Context, command contactport.SidebarProfileUpdateCommand) (contactport.SidebarProfile, error) {
	result, err := service.UpdateSidebarProfileWithEffect(ctx, command)
	return result.Profile, err
}

func (service *SidebarProfileService) UpdateSidebarProfileWithEffect(ctx context.Context, command contactport.SidebarProfileUpdateCommand) (contactport.SidebarProfileUpdateResult, error) {
	if !sidebarProfileReady(service) || ctx == nil || !validSidebarProfileCommand(command) {
		return contactport.SidebarProfileUpdateResult{}, contactport.ErrSidebarProfileInvalid
	}
	payload, err := json.Marshal(command)
	if err != nil {
		return contactport.SidebarProfileUpdateResult{}, contactport.ErrSidebarProfileInvalid
	}
	now := service.now().UTC().Truncate(time.Microsecond)
	if now.IsZero() {
		return contactport.SidebarProfileUpdateResult{}, contactport.ErrSidebarProfileUnavailable
	}
	actorID, _ := strconv.ParseInt(strings.TrimPrefix(string(command.Actor), "admin:"), 10, 64)
	reservation := SidebarProfileReceiptReservation{
		ActorScope: "sidebar_customer_profile:actor:" + strconv.FormatInt(actorID, 10),
		KeyDigest:  sha256.Sum256([]byte(command.IdempotencyKey)), PayloadDigest: sha256.Sum256(payload), CreatedAt: now,
	}
	var result contactport.SidebarProfile
	var effectAcceptance SidebarProfileEffectAcceptance
	err = service.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := service.store.ReserveSidebarProfileReceipt(tx, reservation)
		if reserveErr != nil || !validSidebarProfileReceipt(receipt, reservation) {
			return errors.Join(contactport.ErrSidebarProfileUnavailable, reserveErr)
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return contactport.ErrSidebarProfileConflict
		}
		if !owned {
			if receipt.State != "completed" || json.Unmarshal(receipt.ResultSnapshot, &result) != nil || !validSidebarProfile(result, command.CustomerID, command.OwnerStaffID) {
				return contactport.ErrSidebarProfileUnavailable
			}
			return service.queueSidebarProfileEffect(tx, receipt.ID, actorID, command, result, &effectAcceptance)
		}
		current, readErr := service.store.ReadSidebarProfile(tx, int64(command.CustomerID), command.OwnerStaffID)
		if readErr != nil {
			return readErr
		}
		profileObject, mergeErr := mergeSidebarProfileObject(current.Extra, command.Patch)
		if mergeErr != nil {
			return mergeErr
		}
		current.Extra = profileObject
		current.UpdatedAt = now
		updated, updateErr := service.store.UpdateSidebarProfile(tx, current, command.ExpectedUpdatedAt.UTC().Truncate(time.Microsecond))
		if updateErr != nil {
			return updateErr
		}
		result, updateErr = sidebarProfileProjection(updated)
		if updateErr != nil {
			return updateErr
		}
		eventPayload, marshalErr := json.Marshal(struct {
			CustomerID   contactport.CustomerID `json:"customer_id"`
			OwnerStaffID int64                  `json:"owner_staff_id"`
		}{command.CustomerID, command.OwnerStaffID})
		if marshalErr != nil {
			return marshalErr
		}
		if _, appendErr := service.events.Append(tx, eventport.Event{
			Type: eventport.EvCustomerUpdated, CustomerID: eventport.CustomerID(command.CustomerID), Payload: eventPayload,
			OccurredAt: now, IdempotencyKey: "sidebar.customer_profile.updated:" + strconv.FormatInt(receipt.ID, 10),
		}); appendErr != nil {
			return appendErr
		}
		if effectErr := service.queueSidebarProfileEffect(tx, receipt.ID, actorID, command, result, &effectAcceptance); effectErr != nil {
			return effectErr
		}
		snapshot, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return marshalErr
		}
		completed, completeErr := service.store.CompleteSidebarProfileReceipt(tx, receipt.ID, snapshot, now)
		if completeErr != nil || !validSidebarProfileReceipt(completed, reservation) ||
			subtle.ConstantTimeCompare(completed.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 || completed.State != "completed" {
			return errors.Join(contactport.ErrSidebarProfileUnavailable, completeErr)
		}
		return nil
	})
	if err != nil {
		return contactport.SidebarProfileUpdateResult{}, sidebarProfileError(err)
	}
	return contactport.SidebarProfileUpdateResult{
		Profile: result, EffectQueued: effectAcceptance.Queued,
		ProviderExecutionEligible: effectAcceptance.ProviderExecutionEligible,
	}, nil
}

func (service *SidebarProfileService) queueSidebarProfileEffect(ctx context.Context, receiptID, actorID int64, command contactport.SidebarProfileUpdateCommand, profile contactport.SidebarProfile, acceptance *SidebarProfileEffectAcceptance) error {
	if service.effect == nil {
		return nil
	}
	queued, err := service.effect.QueueInTransaction(ctx, SidebarProfileEffectCommand{
		ReceiptID: receiptID, ActorID: actorID, CustomerID: command.CustomerID,
		IdempotencyKey: command.IdempotencyKey, Profile: profile,
	})
	if err != nil || !queued.Queued || !queued.ProviderExecutionEligible {
		return errors.Join(contactport.ErrSidebarProfileUnavailable, err)
	}
	*acceptance = queued
	return nil
}

func mergeSidebarProfileObject(extra json.RawMessage, patch contactport.SidebarProfilePatch) (json.RawMessage, error) {
	var root map[string]json.RawMessage
	if !IsChannelNeutralCustomerExtra(extra) || json.Unmarshal(extra, &root) != nil {
		return nil, contactport.ErrSidebarProfileUnavailable
	}
	profile := map[string]string{}
	if raw := root["sidebar_profile"]; len(raw) > 0 && json.Unmarshal(raw, &profile) != nil {
		return nil, contactport.ErrSidebarProfileUnavailable
	}
	for key, value := range map[string]*string{
		"source": patch.Source, "industry": patch.Industry, "description": patch.Description,
		"needs": patch.Needs, "pain_points": patch.PainPoints,
	} {
		if value != nil {
			profile[key] = *value
		}
	}
	raw, err := json.Marshal(profile)
	if err != nil {
		return nil, contactport.ErrSidebarProfileUnavailable
	}
	return raw, nil
}

func sidebarProfileProjection(record SidebarProfileRecord) (contactport.SidebarProfile, error) {
	if record.CustomerID < 1 || record.OwnerStaffID < 1 || record.Name == "" || record.UpdatedAt.IsZero() || !IsChannelNeutralCustomerExtra(record.Extra) {
		return contactport.SidebarProfile{}, contactport.ErrSidebarProfileUnavailable
	}
	var root map[string]json.RawMessage
	var profile map[string]string
	if json.Unmarshal(record.Extra, &root) != nil {
		return contactport.SidebarProfile{}, contactport.ErrSidebarProfileUnavailable
	}
	if raw := root["sidebar_profile"]; len(raw) > 0 && json.Unmarshal(raw, &profile) != nil {
		return contactport.SidebarProfile{}, contactport.ErrSidebarProfileUnavailable
	}
	result := contactport.SidebarProfile{
		CustomerID: contactport.CustomerID(record.CustomerID), Name: record.Name, OwnerStaffID: record.OwnerStaffID,
		Source: profile["source"], Industry: profile["industry"], Description: profile["description"],
		Needs: profile["needs"], PainPoints: profile["pain_points"], UpdatedAt: record.UpdatedAt.UTC(),
	}
	if !validSidebarProfile(result, result.CustomerID, result.OwnerStaffID) {
		return contactport.SidebarProfile{}, contactport.ErrSidebarProfileUnavailable
	}
	return result, nil
}

func validSidebarProfileCommand(command contactport.SidebarProfileUpdateCommand) bool {
	if command.CustomerID < 1 || command.OwnerStaffID < 1 || command.ExpectedUpdatedAt.IsZero() ||
		!strings.HasPrefix(string(command.Actor), "admin:") || len(command.IdempotencyKey) < 16 || len(command.IdempotencyKey) > 128 || strings.TrimSpace(command.IdempotencyKey) != command.IdempotencyKey {
		return false
	}
	actorID, err := strconv.ParseInt(strings.TrimPrefix(string(command.Actor), "admin:"), 10, 64)
	if err != nil || actorID < 1 {
		return false
	}
	count := 0
	for value, maximum := range map[*string]int{command.Patch.Source: 200, command.Patch.Industry: 200, command.Patch.Description: 2000, command.Patch.Needs: 2000, command.Patch.PainPoints: 2000} {
		if value == nil {
			continue
		}
		count++
		if !utf8.ValidString(*value) || strings.TrimSpace(*value) != *value || utf8.RuneCountInString(*value) > maximum {
			return false
		}
	}
	return count > 0
}

func validSidebarProfile(profile contactport.SidebarProfile, customerID contactport.CustomerID, ownerStaffID int64) bool {
	return profile.CustomerID == customerID && profile.OwnerStaffID == ownerStaffID && profile.Name != "" && !profile.UpdatedAt.IsZero()
}

func validSidebarProfileReceipt(receipt SidebarProfileReceipt, reservation SidebarProfileReceiptReservation) bool {
	return receipt.ID > 0 && receipt.ActorScope == reservation.ActorScope &&
		subtle.ConstantTimeCompare(receipt.KeyDigest[:], reservation.KeyDigest[:]) == 1 &&
		(receipt.State == "reserved" || receipt.State == "completed")
}

func sidebarProfileReady(service *SidebarProfileService) bool {
	return service != nil && service.uow != nil && service.store != nil && service.events != nil && service.now != nil
}

func sidebarProfileError(err error) error {
	switch {
	case errors.Is(err, contactport.ErrSidebarProfileInvalid), errors.Is(err, contactport.ErrSidebarProfileNotFound),
		errors.Is(err, contactport.ErrSidebarProfileConflict), errors.Is(err, contactport.ErrSidebarProfileUnavailable):
		return err
	default:
		return errors.Join(contactport.ErrSidebarProfileUnavailable, err)
	}
}

var _ contactport.SidebarProfileService = (*SidebarProfileService)(nil)
