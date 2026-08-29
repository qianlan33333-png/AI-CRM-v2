package v1domain

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	referencehistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1contactreferencehistory"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

func TestContactReferenceHistoryMapsAllPrivateFactsWithoutResolution(t *testing.T) {
	key := []byte(strings.Repeat("k", sha256.Size))
	at := time.Date(2026, 8, 29, 9, 2, 3, 123456789, time.FixedZone("+8", 8*3600))
	resolver := contactReferenceResolverStub{}
	binding, err := contactReferenceBindingFact(key, contactReferenceBindingFixture(at), resolver, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	binding.ID = 1
	if _, err = contactapp.HistoricalExternalContactBindingDigest(binding); err != nil || binding.SourcePersonID != -11 || binding.PersonHistoryID != nil || binding.IdentityID != nil || binding.IdentityAssurance != "unresolved" || binding.CreatedAt.Location() != time.UTC || binding.CreatedAt.Nanosecond() != 123456000 {
		t.Fatalf("binding=%+v err=%v", binding, err)
	}
	if binding.ExternalUserIDDigest != expectedContactReferencePrivateDigest(key, referencehistory.ExternalContactBindingsTableID, "external_userid", "external-secret") || binding.FirstBoundByUserIDDigest == binding.LastOwnerUserIDDigest {
		t.Fatal("binding private field HMAC mapping lost domain separation")
	}

	directoryFact := contactReferenceDirectoryFixture(at)
	directory, err := contactReferenceDirectoryFact(key, directoryFact, resolver, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	directory.ID = 1
	if _, err = contactapp.HistoricalWeComDirectoryMemberDigest(directory); err != nil || directory.SourceID != -12 || directory.MatchedStaffID != nil || directory.WeComStatus != nil || directory.DisplayName != "" || directory.DepartmentName != "" || directory.Position != "" || directory.IsActive || directory.CorpAttribution != "unattributable" || directory.SyncedAt.Location() != time.UTC || directory.SyncedAt.Nanosecond() != 123456000 {
		t.Fatalf("directory=%+v err=%v", directory, err)
	}
	if directory.RawPayloadDigest != expectedContactReferencePrivateDigest(key, referencehistory.AdminWeComDirectoryMembersTableID, "raw_payload_json", "private-json") || directory.MobileDigest == directory.AvatarURLDigest {
		t.Fatal("directory private fields were not separately HMACed")
	}
	encoded, err := json.Marshal(directory)
	if err != nil || strings.Contains(string(encoded), "private-json") || strings.Contains(string(encoded), "mobile-secret") || strings.Contains(string(encoded), "wecom-secret") {
		t.Fatalf("private raw value leaked: %s err=%v", encoded, err)
	}
}

func TestContactReferenceEntriesScopeSortAndRejectInvalidResolver(t *testing.T) {
	key := []byte(strings.Repeat("k", sha256.Size))
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	selection := referencehistory.Selection{Bindings: []referencehistory.BindingCandidate{{SourceOrdinal: 1, Fact: contactReferenceBindingFixture(at)}}, DirectoryMembers: []referencehistory.DirectoryMemberCandidate{{SourceOrdinal: 1, Fact: contactReferenceDirectoryFixture(at)}}}
	entries, err := contactReferenceEntries(context.Background(), selection, "v1-full-archive-20260827", contactReferenceTxStub{}, key, contactReferenceResolverStub{})
	if err != nil || len(entries) != 2 || entries[0].scope.TableID != referencehistory.AdminWeComDirectoryMembersTableID || entries[0].scope.TargetTable != "contact_v1_directory_member_history" || entries[1].scope.TableID != referencehistory.ExternalContactBindingsTableID || entries[1].scope.TargetTable != "contact_v1_external_binding_history" {
		t.Fatalf("entries=%+v err=%v", entries, err)
	}
	if entries[0].kind != contactReferenceDirectoryKind || entries[1].kind != contactReferenceBindingKind || entries[0].source != SourceIdentifier(entries[0].key) || entries[1].payload == ([32]byte{}) || entries[1].field == ([32]byte{}) {
		t.Fatal("entry envelope or kind lost")
	}
	if _, err = contactReferenceEntries(context.Background(), selection, "run", contactReferenceTxStub{}, key, contactReferenceResolverStub{binding: ContactBindingReferences{IdentityAssurance: "declared"}}); err == nil {
		t.Fatal("invalid resolver assurance accepted")
	}
	if _, err = contactReferenceEntries(context.Background(), selection, "run", nil, key, contactReferenceResolverStub{}); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("nil tx err=%v", err)
	}
}

func TestContactReferenceHistoryJournalRejectsMalformedReplay(t *testing.T) {
	bridge := contactReferenceHistoryJournal{journal: &Journal{scope: Scope{ImportVersion: ContactReferenceHistoryVersion, ArchiveRunID: "run", AdapterID: "v1_full_archive", TableID: referencehistory.ExternalContactBindingsTableID, TargetDomain: "contact", TargetTable: "contact_v1_external_binding_history"}}, kind: contactReferenceBindingKind}
	if _, _, err := bridge.LoadReferenceHistory(context.Background(), contactReferenceDirectoryKind, SourceIdentifier(contactReferenceDigest("source"))); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("wrong kind err=%v", err)
	}
	if err := bridge.RecordReferenceHistory(context.Background(), contactport.ReferenceHistoryReceipt{Kind: contactReferenceBindingKind, SourceIdentifier: SourceIdentifier(contactReferenceDigest("source")), PayloadDigest: contactReferenceDigest("payload"), TargetDigest: contactReferenceDigest("target"), TargetID: 1, Replayed: true}); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("replayed record err=%v", err)
	}
	if err := bridge.RecordReferenceHistory(context.Background(), contactport.ReferenceHistoryReceipt{Kind: contactReferenceBindingKind, SourceIdentifier: "bad", PayloadDigest: contactReferenceDigest("payload"), TargetDigest: contactReferenceDigest("target"), TargetID: 1}); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("malformed source err=%v", err)
	}
}

type contactReferenceResolverStub struct {
	binding   ContactBindingReferences
	directory ContactDirectoryReferences
	err       error
}

func (s contactReferenceResolverStub) ResolveBinding(context.Context, referencehistory.ExternalContactBindingFact) (ContactBindingReferences, error) {
	if s.binding.IdentityAssurance == "" && s.err == nil {
		s.binding.IdentityAssurance = "unresolved"
	}
	return s.binding, s.err
}
func (s contactReferenceResolverStub) ResolveDirectory(context.Context, referencehistory.DirectoryMemberFact) (ContactDirectoryReferences, error) {
	if s.directory.CorpAttribution == "" && s.err == nil {
		s.directory.CorpAttribution = "unattributable"
	}
	return s.directory, s.err
}

type contactReferenceTxStub struct{}

func (contactReferenceTxStub) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("unused")
}
func (contactReferenceTxStub) Commit(context.Context) error   { return errors.New("unused") }
func (contactReferenceTxStub) Rollback(context.Context) error { return errors.New("unused") }
func (contactReferenceTxStub) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("unused")
}
func (contactReferenceTxStub) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (contactReferenceTxStub) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (contactReferenceTxStub) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("unused")
}
func (contactReferenceTxStub) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unused")
}
func (contactReferenceTxStub) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unused")
}
func (contactReferenceTxStub) QueryRow(context.Context, string, ...any) pgx.Row { return nil }
func (contactReferenceTxStub) Conn() *pgx.Conn                                  { return nil }

func contactReferenceBindingFixture(at time.Time) referencehistory.ExternalContactBindingFact {
	return referencehistory.ExternalContactBindingFact{Source: contactReferenceEnvelope(1), ExternalUserID: "external-secret", PersonID: -11, FirstBoundByUserID: "bound-secret", FirstOwnerUserID: "first-owner-secret", LastOwnerUserID: "last-owner-secret", CreatedAt: at, UpdatedAt: at.Add(-time.Second)}
}
func contactReferenceDirectoryFixture(at time.Time) referencehistory.DirectoryMemberFact {
	return referencehistory.DirectoryMemberFact{Source: contactReferenceEnvelope(10), ID: -12, WeComCorpID: "corp-secret", WeComUserID: "wecom-secret", DisplayName: "", DepartmentIDsJSON: "[]", Position: "", WeComStatus: nil, IsActive: false, SyncedAt: at, RawPayloadJSON: "private-json", CreatedAt: at, UpdatedAt: at.Add(-time.Second), CorpID: "corp-alias-secret", DepartmentName: "", Mobile: "mobile-secret", AvatarURL: "avatar-secret", FirstSeenAt: at, LastSyncedAt: at.Add(-time.Second), UpdatedBy: "updater-secret"}
}
func contactReferenceEnvelope(seed byte) referencehistory.SourceEnvelope {
	return referencehistory.SourceEnvelope{SourceKeyDigest: contactReferenceDigest(string([]byte{seed, 1})), PayloadDigest: contactReferenceDigest(string([]byte{seed, 2})), FieldDigest: contactReferenceDigest(string([]byte{seed, 3}))}
}
func contactReferenceDigest(value string) [sha256.Size]byte { return sha256.Sum256([]byte(value)) }
func expectedContactReferencePrivateDigest(key []byte, table, field, value string) [sha256.Size]byte {
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
