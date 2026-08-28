package v1domain

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"reflect"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	referencehistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1contactreferencehistory"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const ContactReferenceHistoryVersion = "v1-contact-reference-history-a1"

const (
	contactReferenceBindingKind   = "external_contact_binding"
	contactReferenceDirectoryKind = "wecom_directory_member"
)

type ContactReferenceHistoryResult struct {
	Selected, Imported, Replayed int
	Reconciliation               *ReconciliationResult `json:",omitempty"`
}

// ContactReferenceResolver may return only pre-existing, nullable historical
// references. It must not create a Customer, Staff, identity, or assurance.
type ContactReferenceResolver interface {
	ResolveBinding(context.Context, referencehistory.ExternalContactBindingFact) (ContactBindingReferences, error)
	ResolveDirectory(context.Context, referencehistory.DirectoryMemberFact) (ContactDirectoryReferences, error)
}

type ContactBindingReferences struct {
	PersonHistoryID, IdentityID *int64
	IdentityAssurance           string
}

type ContactDirectoryReferences struct {
	MatchedStaffID  *int64
	CorpAttribution string
}

type contactReferenceHistoryJournal struct {
	journal *Journal
	kind    string
}

func (j contactReferenceHistoryJournal) LoadReferenceHistory(ctx context.Context, kind, source string) (contactport.ReferenceHistoryReceipt, bool, error) {
	if j.journal == nil || j.journal.scope.ImportVersion != ContactReferenceHistoryVersion || kind != j.kind {
		return contactport.ReferenceHistoryReceipt{}, false, ErrInvalidScope
	}
	value, found, err := j.journal.LoadTerminal(ctx, source)
	if err != nil || !found {
		return contactport.ReferenceHistoryReceipt{}, found, err
	}
	id, err := positiveID(value.TargetID)
	if err != nil || value.TargetID != strconv.FormatInt(id, 10) || SourceIdentifier(value.SourceKeyDigest) != source || value.Disposition != "import" || value.Reason != "" || len(value.Metadata) != 0 || value.PayloadDigest == ([32]byte{}) || value.TargetDigest == ([32]byte{}) {
		return contactport.ReferenceHistoryReceipt{}, false, ErrConflict
	}
	return contactport.ReferenceHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: value.PayloadDigest, TargetID: id, TargetDigest: value.TargetDigest}, true, nil
}

func (j contactReferenceHistoryJournal) RecordReferenceHistory(ctx context.Context, value contactport.ReferenceHistoryReceipt) error {
	key, err := ParseSourceIdentifier(value.SourceIdentifier)
	if j.journal == nil || j.journal.scope.ImportVersion != ContactReferenceHistoryVersion || err != nil || key == ([32]byte{}) || value.SourceIdentifier != SourceIdentifier(key) || value.Kind != j.kind || value.Replayed || value.TargetID < 1 || value.PayloadDigest == ([32]byte{}) || value.TargetDigest == ([32]byte{}) {
		return ErrInvalidScope
	}
	return j.journal.Record(ctx, TerminalReceipt{SourceKeyDigest: key, PayloadDigest: value.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(value.TargetID, 10), TargetDigest: value.TargetDigest})
}

type contactReferenceEntry struct {
	scope               Scope
	kind, source        string
	key, payload, field [32]byte
	journal             *Journal
	write               func(context.Context) (contactport.ReferenceHistoryReceipt, error)
	verify              func(context.Context, int64) ([32]byte, error)
}

func contactReferenceEntries(ctx context.Context, selected referencehistory.Selection, run string, tx pgx.Tx, key []byte, resolver ContactReferenceResolver) ([]contactReferenceEntry, error) {
	if ctx == nil || run == "" || tx == nil || len(key) < sha256.Size || nilContactReferenceResolver(resolver) {
		return nil, ErrInvalidScope
	}
	entries := make([]contactReferenceEntry, 0, selected.Total())
	reader := contactstore.NewReferenceHistoryReader(tx)
	for _, source := range selected.Bindings {
		fact, err := contactReferenceBindingFact(key, source.Fact, resolver, ctx)
		if err != nil {
			return nil, err
		}
		scope := Scope{ImportVersion: ContactReferenceHistoryVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: referencehistory.ExternalContactBindingsTableID, TargetDomain: "contact", TargetTable: "contact_v1_external_binding_history"}
		journal, err := NewJournal(scope)
		if err != nil {
			return nil, err
		}
		writer, err := contactapp.NewReferenceHistoryWriter(contactstore.NewReferenceHistoryStore(), contactReferenceHistoryJournal{journal: journal, kind: contactReferenceBindingKind})
		if err != nil {
			return nil, err
		}
		probe := fact
		probe.ID = 1
		if _, err = contactapp.HistoricalExternalContactBindingDigest(probe); err != nil {
			return nil, err
		}
		sourceID := SourceIdentifier(fact.SourceKeyDigest)
		entries = append(entries, contactReferenceEntry{scope: scope, kind: contactReferenceBindingKind, source: sourceID, key: fact.SourceKeyDigest, payload: fact.SourcePayloadDigest, field: fact.SourceFieldDigest, journal: journal,
			write: func(ctx context.Context) (contactport.ReferenceHistoryReceipt, error) {
				return writer.ImportHistoricalExternalContactBinding(ctx, sourceID, fact)
			},
			verify: func(ctx context.Context, id int64) ([32]byte, error) {
				actual, err := reader.GetHistoricalExternalContactBinding(ctx, id)
				if err != nil || actual.ID != id {
					return [32]byte{}, ErrConflict
				}
				expected := fact
				expected.ID = id
				want, wantErr := contactapp.HistoricalExternalContactBindingDigest(expected)
				got, gotErr := contactapp.HistoricalExternalContactBindingDigest(actual)
				if wantErr != nil || gotErr != nil || want != got {
					return [32]byte{}, ErrConflict
				}
				return got, nil
			},
		})
	}
	for _, source := range selected.DirectoryMembers {
		fact, err := contactReferenceDirectoryFact(key, source.Fact, resolver, ctx)
		if err != nil {
			return nil, err
		}
		scope := Scope{ImportVersion: ContactReferenceHistoryVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: referencehistory.AdminWeComDirectoryMembersTableID, TargetDomain: "contact", TargetTable: "contact_v1_directory_member_history"}
		journal, err := NewJournal(scope)
		if err != nil {
			return nil, err
		}
		writer, err := contactapp.NewReferenceHistoryWriter(contactstore.NewReferenceHistoryStore(), contactReferenceHistoryJournal{journal: journal, kind: contactReferenceDirectoryKind})
		if err != nil {
			return nil, err
		}
		probe := fact
		probe.ID = 1
		if _, err = contactapp.HistoricalWeComDirectoryMemberDigest(probe); err != nil {
			return nil, err
		}
		sourceID := SourceIdentifier(fact.SourceKeyDigest)
		entries = append(entries, contactReferenceEntry{scope: scope, kind: contactReferenceDirectoryKind, source: sourceID, key: fact.SourceKeyDigest, payload: fact.SourcePayloadDigest, field: fact.SourceFieldDigest, journal: journal,
			write: func(ctx context.Context) (contactport.ReferenceHistoryReceipt, error) {
				return writer.ImportHistoricalWeComDirectoryMember(ctx, sourceID, fact)
			},
			verify: func(ctx context.Context, id int64) ([32]byte, error) {
				actual, err := reader.GetHistoricalWeComDirectoryMember(ctx, id)
				if err != nil || actual.ID != id {
					return [32]byte{}, ErrConflict
				}
				expected := fact
				expected.ID = id
				want, wantErr := contactapp.HistoricalWeComDirectoryMemberDigest(expected)
				got, gotErr := contactapp.HistoricalWeComDirectoryMemberDigest(actual)
				if wantErr != nil || gotErr != nil || want != got {
					return [32]byte{}, ErrConflict
				}
				return got, nil
			},
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].scope.TableID+"/"+entries[i].source < entries[j].scope.TableID+"/"+entries[j].source
	})
	for i, entry := range entries {
		if entry.source != SourceIdentifier(entry.key) || entry.key == ([32]byte{}) || entry.payload == ([32]byte{}) || entry.field == ([32]byte{}) || (i > 0 && entries[i-1].scope.TableID == entry.scope.TableID && entries[i-1].source == entry.source) {
			return nil, ErrConflict
		}
	}
	return entries, nil
}

func contactReferenceBindingFact(key []byte, value referencehistory.ExternalContactBindingFact, resolver ContactReferenceResolver, ctx context.Context) (contactport.HistoricalExternalContactBinding, error) {
	references, err := resolver.ResolveBinding(ctx, value)
	if err != nil {
		return contactport.HistoricalExternalContactBinding{}, err
	}
	return contactport.HistoricalExternalContactBinding{
		SourceKeyDigest: value.Source.SourceKeyDigest, SourcePayloadDigest: value.Source.PayloadDigest, SourceFieldDigest: value.Source.FieldDigest,
		ExternalUserIDDigest: contactReferencePrivateDigest(key, referencehistory.ExternalContactBindingsTableID, "external_userid", value.ExternalUserID), SourcePersonID: value.PersonID,
		PersonHistoryID: cloneContactReferenceID(references.PersonHistoryID), IdentityID: cloneContactReferenceID(references.IdentityID), IdentityAssurance: references.IdentityAssurance,
		FirstBoundByUserIDDigest: contactReferencePrivateDigest(key, referencehistory.ExternalContactBindingsTableID, "first_bound_by_userid", value.FirstBoundByUserID),
		FirstOwnerUserIDDigest:   contactReferencePrivateDigest(key, referencehistory.ExternalContactBindingsTableID, "first_owner_userid", value.FirstOwnerUserID),
		LastOwnerUserIDDigest:    contactReferencePrivateDigest(key, referencehistory.ExternalContactBindingsTableID, "last_owner_userid", value.LastOwnerUserID),
		CreatedAt:                value.CreatedAt.UTC().Truncate(time.Microsecond), UpdatedAt: value.UpdatedAt.UTC().Truncate(time.Microsecond),
	}, nil
}

func contactReferenceDirectoryFact(key []byte, value referencehistory.DirectoryMemberFact, resolver ContactReferenceResolver, ctx context.Context) (contactport.HistoricalWeComDirectoryMember, error) {
	references, err := resolver.ResolveDirectory(ctx, value)
	if err != nil {
		return contactport.HistoricalWeComDirectoryMember{}, err
	}
	return contactport.HistoricalWeComDirectoryMember{
		SourceKeyDigest: value.Source.SourceKeyDigest, SourcePayloadDigest: value.Source.PayloadDigest, SourceFieldDigest: value.Source.FieldDigest, SourceID: value.ID,
		WeComCorpIDDigest: contactReferencePrivateDigest(key, referencehistory.AdminWeComDirectoryMembersTableID, "wecom_corpid", value.WeComCorpID), CorpIDDigest: contactReferencePrivateDigest(key, referencehistory.AdminWeComDirectoryMembersTableID, "corp_id", value.CorpID), WeComUserIDDigest: contactReferencePrivateDigest(key, referencehistory.AdminWeComDirectoryMembersTableID, "wecom_userid", value.WeComUserID),
		CorpAttribution: references.CorpAttribution, MatchedStaffID: cloneContactReferenceID(references.MatchedStaffID), DisplayName: value.DisplayName, DepartmentIDsDigest: contactReferencePrivateDigest(key, referencehistory.AdminWeComDirectoryMembersTableID, "department_ids_json", value.DepartmentIDsJSON), DepartmentName: value.DepartmentName, Position: value.Position, WeComStatus: cloneContactReferenceInt32(value.WeComStatus), IsActive: value.IsActive, SyncedAt: value.SyncedAt.UTC().Truncate(time.Microsecond),
		RawPayloadDigest: contactReferencePrivateDigest(key, referencehistory.AdminWeComDirectoryMembersTableID, "raw_payload_json", value.RawPayloadJSON), MobileDigest: contactReferencePrivateDigest(key, referencehistory.AdminWeComDirectoryMembersTableID, "mobile", value.Mobile), AvatarURLDigest: contactReferencePrivateDigest(key, referencehistory.AdminWeComDirectoryMembersTableID, "avatar_url", value.AvatarURL), UpdatedByDigest: contactReferencePrivateDigest(key, referencehistory.AdminWeComDirectoryMembersTableID, "updated_by", value.UpdatedBy),
		FirstSeenAt: value.FirstSeenAt.UTC().Truncate(time.Microsecond), LastSyncedAt: value.LastSyncedAt.UTC().Truncate(time.Microsecond), CreatedAt: value.CreatedAt.UTC().Truncate(time.Microsecond), UpdatedAt: value.UpdatedAt.UTC().Truncate(time.Microsecond),
	}, nil
}

func contactReferencePrivateDigest(key []byte, table, field, value string) [sha256.Size]byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(ContactReferenceHistoryVersion))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(table))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(field))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(value))
	var result [sha256.Size]byte
	copy(result[:], mac.Sum(nil))
	return result
}

func cloneContactReferenceID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func cloneContactReferenceInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func nilContactReferenceResolver(value ContactReferenceResolver) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Ptr || reflected.Kind() == reflect.Interface) && reflected.IsNil()
}
