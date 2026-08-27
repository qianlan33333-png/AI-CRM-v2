package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"strconv"
	"strings"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const historicalContactTagTaggedBy = "migration:v1-contact-tags"

// HistoricalTagImportService imports only static local tag facts. The
// main-line migration composition supplies source reading and the durable
// receipt journal; this leaf never emits events or executes an external call.
type HistoricalTagImportService struct {
	uow       platformport.UnitOfWork
	store     contactport.HistoricalTagStore
	journal   contactport.HistoricalTagJournal
	customers contactport.HistoricalTagCustomerVerifier
}

func NewHistoricalTagImportService(uow platformport.UnitOfWork, store contactport.HistoricalTagStore, journal contactport.HistoricalTagJournal, customers contactport.HistoricalTagCustomerVerifier) *HistoricalTagImportService {
	return &HistoricalTagImportService{uow: uow, store: store, journal: journal, customers: customers}
}

type HistoricalTagImportResult struct {
	TargetID                  int64
	Replayed                  bool
	LocalProjection           bool
	ProviderExecutionEligible bool
	RealExternalCallExecuted  bool
}

func (service *HistoricalTagImportService) ImportGroup(ctx context.Context, record contactport.HistoricalTagGroupRecord) (HistoricalTagImportResult, error) {
	if !validHistoricalTagFact(record.Fact) || !validHistoricalTagText(record.Name) {
		return HistoricalTagImportResult{}, contactport.ErrHistoricalTagInput
	}
	if !historicalTagReady(service, false) {
		return HistoricalTagImportResult{}, contactport.ErrHistoricalTagUnavailable
	}
	return service.withLineage(ctx, contactport.HistoricalTagGroupSource, record.Fact, func(tx context.Context, lineage contactport.HistoricalTagLineage, found bool) (int64, [32]byte, bool, error) {
		if found {
			group, err := service.store.GetHistoricalTagGroup(tx, lineage.TargetID)
			if err != nil || !sameHistoricalTagGroup(group, record) {
				return 0, [32]byte{}, false, contactport.ErrHistoricalTagConflict
			}
			return group.ID, historicalTagTargetDigest(contactport.HistoricalTagGroupSource, group.ID), true, nil
		}
		group, err := service.store.CreateHistoricalTagGroup(tx, contactport.HistoricalTagGroup{Name: strings.TrimSpace(record.Name), SortOrder: record.SortOrder})
		if err != nil || !sameHistoricalTagGroup(group, record) {
			return 0, [32]byte{}, false, contactport.ErrHistoricalTagUnavailable
		}
		return group.ID, historicalTagTargetDigest(contactport.HistoricalTagGroupSource, group.ID), false, nil
	})
}

func (service *HistoricalTagImportService) ImportTag(ctx context.Context, record contactport.HistoricalTagRecord) (HistoricalTagImportResult, error) {
	if !validHistoricalTagFact(record.Fact) || zeroHistoricalTagDigest(record.GroupSourceKeyDigest) || !validHistoricalProviderTagID(record.ProviderTagID) || !validHistoricalTagText(record.Name) {
		return HistoricalTagImportResult{}, contactport.ErrHistoricalTagInput
	}
	if !historicalTagReady(service, false) {
		return HistoricalTagImportResult{}, contactport.ErrHistoricalTagUnavailable
	}
	return service.withLineage(ctx, contactport.HistoricalTagCatalogTagSource, record.Fact, func(tx context.Context, lineage contactport.HistoricalTagLineage, found bool) (int64, [32]byte, bool, error) {
		groupLineage, groupFound, err := service.journal.FindHistoricalTagLineage(tx, contactport.HistoricalTagGroupSource, record.GroupSourceKeyDigest)
		if err != nil {
			return 0, [32]byte{}, false, err
		}
		if !groupFound || groupLineage.TargetID < 1 || groupLineage.TargetDigest != historicalTagTargetDigest(contactport.HistoricalTagGroupSource, groupLineage.TargetID) {
			return 0, [32]byte{}, false, contactport.ErrHistoricalTagBlocked
		}
		if _, err = service.store.GetHistoricalTagGroup(tx, groupLineage.TargetID); err != nil {
			return 0, [32]byte{}, false, contactport.ErrHistoricalTagBlocked
		}
		if found {
			tag, getErr := service.store.GetHistoricalTag(tx, lineage.TargetID)
			if getErr != nil || !sameHistoricalTag(tag, groupLineage.TargetID, record) {
				return 0, [32]byte{}, false, contactport.ErrHistoricalTagConflict
			}
			return tag.ID, historicalTagTargetDigest(contactport.HistoricalTagCatalogTagSource, tag.ID), true, nil
		}
		tag, _, createErr := service.store.CreateHistoricalTag(tx, contactport.HistoricalTag{GroupID: groupLineage.TargetID, ProviderTagID: strings.TrimSpace(record.ProviderTagID), Name: strings.TrimSpace(record.Name), SortOrder: record.SortOrder})
		if createErr != nil || !sameHistoricalTag(tag, groupLineage.TargetID, record) {
			if createErr == nil {
				return 0, [32]byte{}, false, contactport.ErrHistoricalTagConflict
			}
			return 0, [32]byte{}, false, createErr
		}
		return tag.ID, historicalTagTargetDigest(contactport.HistoricalTagCatalogTagSource, tag.ID), false, nil
	})
}

func (service *HistoricalTagImportService) ImportCustomerTag(ctx context.Context, record contactport.HistoricalCustomerTagRecord) (HistoricalTagImportResult, error) {
	if !validHistoricalTagFact(record.Fact) || !validHistoricalUnionID(record.UnionID) || record.VerifiedCustomerID < 1 || !validHistoricalProviderTagID(record.ProviderTagID) || record.TaggedAt.IsZero() {
		return HistoricalTagImportResult{}, contactport.ErrHistoricalTagInput
	}
	if !historicalTagReady(service, true) {
		return HistoricalTagImportResult{}, contactport.ErrHistoricalTagUnavailable
	}
	return service.withLineage(ctx, contactport.HistoricalCustomerTagSource, record.Fact, func(tx context.Context, lineage contactport.HistoricalTagLineage, found bool) (int64, [32]byte, bool, error) {
		if err := service.customers.VerifyHistoricalTagCustomer(tx, strings.TrimSpace(record.UnionID), record.VerifiedCustomerID); err != nil {
			return 0, [32]byte{}, false, contactport.ErrHistoricalTagBlocked
		}
		tag, tagFound, err := service.store.FindHistoricalTagByProviderID(tx, strings.TrimSpace(record.ProviderTagID))
		if err != nil {
			return 0, [32]byte{}, false, err
		}
		if !tagFound || tag.ID < 1 {
			return 0, [32]byte{}, false, contactport.ErrHistoricalTagBlocked
		}
		targetDigest := historicalCustomerTagTargetDigest(record.VerifiedCustomerID, tag.ID)
		if found {
			if lineage.TargetID != tag.ID || lineage.TargetDigest != targetDigest {
				return 0, [32]byte{}, false, contactport.ErrHistoricalTagConflict
			}
			current, currentFound, getErr := service.store.GetHistoricalCustomerTag(tx, record.VerifiedCustomerID, tag.ID)
			if getErr != nil || !currentFound || !sameHistoricalCustomerTag(current, record.VerifiedCustomerID, tag.ID, record.TaggedAt) {
				return 0, [32]byte{}, false, contactport.ErrHistoricalTagConflict
			}
			return tag.ID, targetDigest, true, nil
		}
		bound, _, bindErr := service.store.BindHistoricalCustomerTag(tx, contactport.HistoricalCustomerTag{CustomerID: record.VerifiedCustomerID, TagID: tag.ID, TaggedAt: record.TaggedAt.UTC(), TaggedBy: historicalContactTagTaggedBy})
		if bindErr != nil {
			return 0, [32]byte{}, false, bindErr
		}
		if !sameHistoricalCustomerTag(bound, record.VerifiedCustomerID, tag.ID, record.TaggedAt) {
			return 0, [32]byte{}, false, contactport.ErrHistoricalTagConflict
		}
		return tag.ID, targetDigest, false, nil
	})
}

func (service *HistoricalTagImportService) withLineage(ctx context.Context, source contactport.HistoricalTagSource, fact contactport.HistoricalTagFact, apply func(context.Context, contactport.HistoricalTagLineage, bool) (int64, [32]byte, bool, error)) (HistoricalTagImportResult, error) {
	if ctx == nil || ctx.Err() != nil {
		return HistoricalTagImportResult{}, contactport.ErrHistoricalTagUnavailable
	}
	result := HistoricalTagImportResult{LocalProjection: true}
	err := service.uow.Within(ctx, func(tx context.Context) error {
		lineage, found, err := service.journal.FindHistoricalTagLineage(tx, source, fact.SourceKeyDigest)
		if err != nil {
			return err
		}
		if found && !sameHistoricalTagFact(lineage, fact) {
			return contactport.ErrHistoricalTagConflict
		}
		targetID, targetDigest, replayed, err := apply(tx, lineage, found)
		if err != nil {
			return err
		}
		if targetID < 1 || zeroHistoricalTagDigest(targetDigest) {
			return contactport.ErrHistoricalTagUnavailable
		}
		if found {
			if lineage.TargetID != targetID || lineage.TargetDigest != targetDigest || !replayed {
				return contactport.ErrHistoricalTagConflict
			}
			result.TargetID, result.Replayed = targetID, true
			return nil
		}
		if replayed {
			return contactport.ErrHistoricalTagConflict
		}
		if err = service.journal.AppendHistoricalTagLineage(tx, source, fact, contactport.HistoricalTagLineage{TargetID: targetID, TargetDigest: targetDigest, PayloadDigest: fact.PayloadDigest, FieldDigest: fact.FieldDigest}); err != nil {
			return err
		}
		result.TargetID = targetID
		return nil
	})
	if err != nil {
		return HistoricalTagImportResult{}, classifyHistoricalTagError(err)
	}
	return result, nil
}

func historicalTagReady(service *HistoricalTagImportService, customer bool) bool {
	if service == nil || service.uow == nil || service.store == nil || service.journal == nil {
		return false
	}
	return !customer || service.customers != nil
}

func validHistoricalTagFact(fact contactport.HistoricalTagFact) bool {
	return !zeroHistoricalTagDigest(fact.SourceKeyDigest) && !zeroHistoricalTagDigest(fact.PayloadDigest) && !zeroHistoricalTagDigest(fact.FieldDigest)
}

func zeroHistoricalTagDigest(value [32]byte) bool { return value == [32]byte{} }

func validHistoricalTagText(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len([]rune(value)) <= 200
}

func validHistoricalProviderTagID(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 128
}

func validHistoricalUnionID(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 1024
}

func sameHistoricalTagFact(lineage contactport.HistoricalTagLineage, fact contactport.HistoricalTagFact) bool {
	return lineage.PayloadDigest == fact.PayloadDigest && lineage.FieldDigest == fact.FieldDigest
}

func sameHistoricalTagGroup(value contactport.HistoricalTagGroup, record contactport.HistoricalTagGroupRecord) bool {
	return value.ID > 0 && value.Name == strings.TrimSpace(record.Name) && value.SortOrder == record.SortOrder
}

func sameHistoricalTag(value contactport.HistoricalTag, groupID int64, record contactport.HistoricalTagRecord) bool {
	return value.ID > 0 && value.GroupID == groupID && value.ProviderTagID == strings.TrimSpace(record.ProviderTagID) && value.Name == strings.TrimSpace(record.Name) && value.SortOrder == record.SortOrder
}

func sameHistoricalCustomerTag(value contactport.HistoricalCustomerTag, customerID contactport.CustomerID, tagID int64, taggedAt time.Time) bool {
	return value.CustomerID == customerID && value.TagID == tagID && value.TaggedAt.Equal(taggedAt.UTC()) && value.TaggedBy == historicalContactTagTaggedBy
}

func historicalTagTargetDigest(source contactport.HistoricalTagSource, targetID int64) [32]byte {
	return sha256.Sum256([]byte(string(source) + "\x00" + strconv.FormatInt(targetID, 10)))
}

func historicalCustomerTagTargetDigest(customerID contactport.CustomerID, tagID int64) [32]byte {
	return sha256.Sum256([]byte("v1.contact_tags\x00" + strconv.FormatInt(int64(customerID), 10) + "\x00" + strconv.FormatInt(tagID, 10)))
}

func classifyHistoricalTagError(err error) error {
	if errors.Is(err, contactport.ErrHistoricalTagInput) || errors.Is(err, contactport.ErrHistoricalTagConflict) || errors.Is(err, contactport.ErrHistoricalTagBlocked) {
		return err
	}
	return errors.Join(contactport.ErrHistoricalTagUnavailable, err)
}
