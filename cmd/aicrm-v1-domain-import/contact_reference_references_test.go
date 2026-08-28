package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	referencehistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1contactreferencehistory"
	v1deferredidentityhistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1deferredidentityhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactmigration "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestContactReferenceResolverBindsOnlyVerifiedHistoricalEvidence(t *testing.T) {
	archiveKey := []byte(strings.Repeat("a", sha256.Size))
	dm01Key := []byte(strings.Repeat("d", sha256.Size))
	mapFact := contactReferenceMap{sourceID: -7, corpID: "corp", externalUserID: "external"}
	dm01Source := contactReferenceDM01Source(t, dm01Key, mapFact.sourceID)
	personSource, err := v1archive.SourceKeyHMAC(archiveKey, "people", []byte("[-11]"))
	if err != nil {
		t.Fatal(err)
	}
	person := contactReferenceDeferredPerson(41, -11, personSource, contactReferenceDigest("person-payload"), contactReferenceDigest("person-field"))
	personDigest, err := contactapp.HistoricalDeferredPersonDigest(person)
	if err != nil {
		t.Fatal(err)
	}
	resolver := contactReferenceTestResolver(t, archiveKey, dm01Key, []contactReferenceIdentityMap{mapFact.withDigest()}, contactReferenceTargetFake{
		lineages: map[string]contactport.HistoricalImportLineage{string(dm01Source): {TargetID: 70, PayloadHMAC: contactReferenceBytes("identity-payload"), FieldDigest: contactReferenceBytes("identity-field"), LastRunID: 2}},
		receipts: map[string]contactport.HistoricalImportRowReceipt{string(dm01Source): {PayloadHMAC: contactReferenceBytes("identity-payload"), FieldDigest: contactReferenceBytes("identity-field"), Disposition: contactport.HistoricalImportImported}},
	}, contactReferenceIdentityFake{evidence: identityport.HistoricalScopedWeComIdentityEvidence{IdentityID: 70, Scope: "wecom-corp:corp", ExternalUserID: "external", Assurance: identityport.AssuranceDeclared, HMACKeyVersion: 7}}, contactReferenceStaffFake{}, &contactReferenceTerminalFake{terminal: v1domain.TerminalReceipt{SourceKeyDigest: personSource, PayloadDigest: person.SourcePayloadDigest, Disposition: "import", TargetID: "41", TargetDigest: personDigest}}, person)
	result, err := resolver.ResolveBinding(context.Background(), referencehistory.ExternalContactBindingFact{Source: contactReferenceBindingEnvelope(), ExternalUserID: "external", PersonID: -11})
	if err != nil || result.PersonHistoryID == nil || *result.PersonHistoryID != 41 || result.IdentityID == nil || *result.IdentityID != 70 || result.IdentityAssurance != "declared" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestContactReferenceResolverDoesNotChooseAmbiguousOrDriftedIdentity(t *testing.T) {
	archiveKey := []byte(strings.Repeat("a", sha256.Size))
	dm01Key := []byte(strings.Repeat("d", sha256.Size))
	first, second := contactReferenceMap{sourceID: 1, corpID: "corp", externalUserID: "external"}.withDigest(), contactReferenceMap{sourceID: 2, corpID: "corp", externalUserID: "external"}.withDigest()
	firstKey, secondKey := contactReferenceDM01Source(t, dm01Key, 1), contactReferenceDM01Source(t, dm01Key, 2)
	baseTarget := contactReferenceTargetFake{lineages: map[string]contactport.HistoricalImportLineage{}, receipts: map[string]contactport.HistoricalImportRowReceipt{}}
	for key, target := range map[string]int64{string(firstKey): 70, string(secondKey): 71} {
		baseTarget.lineages[key] = contactport.HistoricalImportLineage{TargetID: target, PayloadHMAC: contactReferenceBytes(key + "payload"), FieldDigest: contactReferenceBytes(key + "field"), LastRunID: 2}
		baseTarget.receipts[key] = contactport.HistoricalImportRowReceipt{PayloadHMAC: contactReferenceBytes(key + "payload"), FieldDigest: contactReferenceBytes(key + "field"), Disposition: contactport.HistoricalImportImported}
	}
	identity := contactReferenceIdentityFake{byID: map[int64]identityport.HistoricalScopedWeComIdentityEvidence{
		70: {IdentityID: 70, Scope: "wecom-corp:corp", ExternalUserID: "external", Assurance: identityport.AssuranceDeclared, HMACKeyVersion: 7},
		71: {IdentityID: 71, Scope: "wecom-corp:corp", ExternalUserID: "external", Assurance: identityport.AssuranceVerified, HMACKeyVersion: 7},
	}}
	resolver := contactReferenceTestResolver(t, archiveKey, dm01Key, []contactReferenceIdentityMap{first, second}, baseTarget, identity, contactReferenceStaffFake{}, &contactReferenceTerminalFake{}, contactport.HistoricalDeferredPerson{})
	result, err := resolver.ResolveBinding(context.Background(), referencehistory.ExternalContactBindingFact{Source: contactReferenceBindingEnvelope(), ExternalUserID: "external"})
	if err != nil || result.IdentityID != nil || result.IdentityAssurance != "unresolved" {
		t.Fatalf("ambiguous result=%+v err=%v", result, err)
	}
	baseTarget.receipts[string(firstKey)] = contactport.HistoricalImportRowReceipt{PayloadHMAC: contactReferenceBytes("drift"), FieldDigest: contactReferenceBytes("field"), Disposition: contactport.HistoricalImportImported}
	resolver = contactReferenceTestResolver(t, archiveKey, dm01Key, []contactReferenceIdentityMap{first}, baseTarget, identity, contactReferenceStaffFake{}, &contactReferenceTerminalFake{}, contactport.HistoricalDeferredPerson{})
	if _, err = resolver.ResolveBinding(context.Background(), referencehistory.ExternalContactBindingFact{Source: contactReferenceBindingEnvelope(), ExternalUserID: "external"}); !errors.Is(err, v1domain.ErrConflict) {
		t.Fatalf("drift error=%v", err)
	}
}

func TestContactReferenceResolverIdentityReceiptStates(t *testing.T) {
	archiveKey := []byte(strings.Repeat("a", sha256.Size))
	dm01Key := []byte(strings.Repeat("d", sha256.Size))
	candidate := contactReferenceMap{sourceID: 1, corpID: "corp", externalUserID: "external"}.withDigest()
	source := contactReferenceDM01Source(t, dm01Key, candidate.sourceID)
	validLineage := contactport.HistoricalImportLineage{TargetID: 70, PayloadHMAC: contactReferenceBytes("payload"), FieldDigest: contactReferenceBytes("field"), LastRunID: 2}
	validImported := contactport.HistoricalImportRowReceipt{PayloadHMAC: contactReferenceBytes("payload"), FieldDigest: contactReferenceBytes("field"), Disposition: contactport.HistoricalImportImported}
	validQuarantine := contactport.HistoricalImportRowReceipt{PayloadHMAC: contactReferenceBytes("payload"), FieldDigest: contactReferenceBytes("field"), Disposition: contactport.HistoricalImportQuarantined}
	for _, test := range []struct {
		name    string
		lineage bool
		receipt *contactport.HistoricalImportRowReceipt
		wantErr bool
	}{
		{name: "no evidence"},
		{name: "quarantined without lineage", receipt: &validQuarantine},
		{name: "imported without lineage", receipt: &validImported, wantErr: true},
		{name: "lineage without receipt", lineage: true, wantErr: true},
		{name: "quarantined with claimed lineage", lineage: true, receipt: &validQuarantine, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			target := contactReferenceTargetFake{lineages: map[string]contactport.HistoricalImportLineage{}, receipts: map[string]contactport.HistoricalImportRowReceipt{}}
			if test.lineage {
				target.lineages[string(source)] = validLineage
			}
			if test.receipt != nil {
				target.receipts[string(source)] = *test.receipt
			}
			resolver := contactReferenceTestResolver(t, archiveKey, dm01Key, []contactReferenceIdentityMap{candidate}, target, contactReferenceIdentityFake{}, contactReferenceStaffFake{}, &contactReferenceTerminalFake{}, contactport.HistoricalDeferredPerson{})
			result, err := resolver.ResolveBinding(context.Background(), referencehistory.ExternalContactBindingFact{Source: contactReferenceBindingEnvelope(), ExternalUserID: "external"})
			if test.wantErr {
				if !errors.Is(err, v1domain.ErrConflict) {
					t.Fatalf("error=%v", err)
				}
				return
			}
			if err != nil || result.IdentityID != nil || result.IdentityAssurance != "unresolved" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestContactReferenceResolverDirectoryRequiresBothCorporations(t *testing.T) {
	resolver := contactReferenceTestResolver(t, []byte(strings.Repeat("a", sha256.Size)), []byte(strings.Repeat("d", sha256.Size)), nil, contactReferenceTargetFake{}, contactReferenceIdentityFake{}, contactReferenceStaffFake{id: 99}, &contactReferenceTerminalFake{}, contactport.HistoricalDeferredPerson{})
	matched, err := resolver.ResolveDirectory(context.Background(), referencehistory.DirectoryMemberFact{Source: contactReferenceBindingEnvelope(), CorpID: "corp", WeComCorpID: "corp", WeComUserID: "staff"})
	if err != nil || matched.CorpAttribution != "matched" || matched.MatchedStaffID == nil || *matched.MatchedStaffID != 99 {
		t.Fatalf("matched=%+v err=%v", matched, err)
	}
	unattributable, err := resolver.ResolveDirectory(context.Background(), referencehistory.DirectoryMemberFact{Source: contactReferenceBindingEnvelope(), CorpID: "other", WeComCorpID: "corp", WeComUserID: "staff"})
	if err != nil || unattributable.CorpAttribution != "unattributable" || unattributable.MatchedStaffID != nil {
		t.Fatalf("unattributable=%+v err=%v", unattributable, err)
	}
	resolver.staff = contactReferenceStaffFake{err: contactport.ErrStaffReferenceNotFound}
	missing, err := resolver.ResolveDirectory(context.Background(), referencehistory.DirectoryMemberFact{Source: contactReferenceBindingEnvelope(), CorpID: "corp", WeComCorpID: "corp", WeComUserID: "staff"})
	if err != nil || missing.CorpAttribution != "matched" || missing.MatchedStaffID != nil {
		t.Fatalf("missing=%+v err=%v", missing, err)
	}
}

func TestLoadContactReferenceIdentityMapsAuthenticatesWholeSource(t *testing.T) {
	key := []byte(strings.Repeat("k", sha256.Size))
	first := contactReferenceIdentityMapRow(t, key, 1, "corp", "external-a")
	second := contactReferenceIdentityMapRow(t, key, -2, "", "")
	second.SourceOrdinal = 2
	archive := contactReferenceArchiveFake{rows: []v1archive.ArchivedRow{first, second}}
	maps, err := loadContactReferenceIdentityMaps(context.Background(), archive, "run", key, 2)
	if err != nil || len(maps["external-a"]) != 1 || len(maps[""]) != 0 {
		t.Fatalf("maps=%v err=%v", maps, err)
	}
	if _, err = loadContactReferenceIdentityMaps(context.Background(), archive, "run", key, 3); !errors.Is(err, v1domain.ErrConflict) {
		t.Fatalf("truncated error=%v", err)
	}
	broken := first
	broken.PayloadHMAC[0]++
	if _, err = loadContactReferenceIdentityMaps(context.Background(), contactReferenceArchiveFake{rows: []v1archive.ArchivedRow{broken}}, "run", key, 1); !errors.Is(err, v1domain.ErrConflict) {
		t.Fatalf("HMAC error=%v", err)
	}
}

func contactReferenceTestResolver(t *testing.T, archiveKey, dm01Key []byte, maps []contactReferenceIdentityMap, contacts contactReferenceTargetFake, identities contactReferenceIdentityFake, staff contactReferenceStaffFake, journal *contactReferenceTerminalFake, person contactport.HistoricalDeferredPerson) *contactReferenceResolver {
	t.Helper()
	indexed := map[string][]contactReferenceIdentityMap{}
	for _, value := range maps {
		indexed[value.externalUserID] = append(indexed[value.externalUserID], value)
	}
	return &contactReferenceResolver{archiveRun: "archive", dm01Run: 2, dm01KeyVersion: 7, archiveKey: archiveKey, dm01Key: dm01Key, corpID: "corp", maps: indexed, contacts: contacts, identities: identities, staff: staff, peopleJournal: journal,
		deferred: func(context.Context) (contactReferenceDeferredReader, error) {
			return contactReferenceDeferredFake{person: person}, nil
		}}
}

type contactReferenceTargetFake struct {
	contactport.HistoricalImportTarget
	lineages map[string]contactport.HistoricalImportLineage
	receipts map[string]contactport.HistoricalImportRowReceipt
	err      error
}

func (f contactReferenceTargetFake) LockHistoricalImportSource(context.Context, contactport.HistoricalImportSource, []byte) error {
	return f.err
}
func (f contactReferenceTargetFake) LockHistoricalImportLineage(_ context.Context, _ contactport.HistoricalImportSource, key []byte) (contactport.HistoricalImportLineage, bool, error) {
	value, found := f.lineages[string(key)]
	return value, found, f.err
}
func (f contactReferenceTargetFake) FindHistoricalImportRowReceipt(_ context.Context, _ int64, _ contactport.HistoricalImportSource, key []byte) (contactport.HistoricalImportRowReceipt, bool, error) {
	value, found := f.receipts[string(key)]
	return value, found, f.err
}

type contactReferenceIdentityFake struct {
	evidence identityport.HistoricalScopedWeComIdentityEvidence
	byID     map[int64]identityport.HistoricalScopedWeComIdentityEvidence
	err      error
}

func (f contactReferenceIdentityFake) LockHistoricalScopedWeComIdentityEvidence(_ context.Context, id int64, _ []byte) (identityport.HistoricalScopedWeComIdentityEvidence, error) {
	if f.err != nil {
		return identityport.HistoricalScopedWeComIdentityEvidence{}, f.err
	}
	if f.byID != nil {
		return f.byID[id], nil
	}
	return f.evidence, nil
}

type contactReferenceStaffFake struct {
	id  int64
	err error
}

func (f contactReferenceStaffFake) LockUniqueActiveStaffForHistoricalImport(context.Context, string) (contactport.HistoricalImportStaff, error) {
	return contactport.HistoricalImportStaff{ID: f.id}, f.err
}

type contactReferenceTerminalFake struct {
	terminal v1domain.TerminalReceipt
	found    bool
	err      error
}

func (f *contactReferenceTerminalFake) LoadTerminal(context.Context, string) (v1domain.TerminalReceipt, bool, error) {
	if f.found || f.terminal.TargetID != "" {
		return f.terminal, true, f.err
	}
	return v1domain.TerminalReceipt{}, false, f.err
}

type contactReferenceDeferredFake struct {
	person contactport.HistoricalDeferredPerson
}

func (f contactReferenceDeferredFake) GetHistoricalDeferredPerson(context.Context, int64) (contactport.HistoricalDeferredPerson, error) {
	if f.person.ID < 1 {
		return contactport.HistoricalDeferredPerson{}, errors.New("missing")
	}
	return f.person, nil
}

type contactReferenceArchiveFake struct{ rows []v1archive.ArchivedRow }

func (f contactReferenceArchiveFake) EachTableRow(_ context.Context, _ string, table string, emit func(v1archive.ArchivedRow) error) error {
	if table != v1deferredidentityhistory.ExternalContactIdentityMapID {
		return errors.New("unexpected table")
	}
	for _, row := range f.rows {
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}

type contactReferenceMap struct {
	sourceID               int64
	corpID, externalUserID string
}

func (v contactReferenceMap) withDigest() contactReferenceIdentityMap {
	return contactReferenceIdentityMap{sourceID: v.sourceID, corpID: v.corpID, externalUserID: v.externalUserID, sourceKey: contactReferenceDigest("source" + strconv.FormatInt(v.sourceID, 10)), payload: contactReferenceDigest("payload" + strconv.FormatInt(v.sourceID, 10)), field: contactReferenceDigest("field" + strconv.FormatInt(v.sourceID, 10))}
}

func contactReferenceBindingEnvelope() referencehistory.SourceEnvelope {
	return referencehistory.SourceEnvelope{SourceKeyDigest: contactReferenceDigest("binding-source"), PayloadDigest: contactReferenceDigest("binding-payload"), FieldDigest: contactReferenceDigest("binding-field")}
}

func contactReferenceDeferredPerson(id, sourceID int64, source, payload, field [sha256.Size]byte) contactport.HistoricalDeferredPerson {
	at := time.Date(2026, 8, 29, 1, 2, 3, 456000, time.UTC)
	return contactport.HistoricalDeferredPerson{ID: id, SourceID: sourceID, SourceKeyDigest: source, SourcePayloadDigest: payload, SourceFieldDigest: field, MobileDigest: contactReferenceDigest("mobile"), ThirdPartyUserIDDigest: contactReferenceDigest("third-party"), PrivateDigest: contactReferenceDigest("private"), RedactedRoots: []string{"mobile"}, CreatedAt: at, UpdatedAt: at}
}

func contactReferenceDM01Source(t *testing.T, key []byte, sourceID int64) []byte {
	t.Helper()
	value, err := contactmigration.SourceKeyHMAC(key, v1deferredidentityhistory.DM01IdentityMapSourceTable, strconv.FormatInt(sourceID, 10))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func contactReferenceBytes(value string) []byte {
	digest := contactReferenceDigest(value)
	return digest[:]
}
func contactReferenceDigest(value string) [sha256.Size]byte { return sha256.Sum256([]byte(value)) }

func contactReferenceIdentityMapRow(t *testing.T, key []byte, id int64, corpID, externalUserID string) v1archive.ArchivedRow {
	t.Helper()
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	raw, err := json.Marshal(map[string]any{
		"id": id, "corp_id": corpID, "external_userid": externalUserID, "unionid": "", "openid": "", "follow_user_userid": "", "name": "", "type": nil, "avatar": "", "gender": nil, "status": "", "raw_profile": nil,
		"first_seen_at": at, "last_seen_at": at, "created_at": at, "updated_at": at,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, roots, err := v1archive.RedactPayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	source, err := v1archive.SourceKeyHMAC(key, "wecom_external_contact_identity_map", []byte("["+strconv.FormatInt(id, 10)+"]"))
	if err != nil {
		t.Fatal(err)
	}
	payloadHMAC, err := v1archive.PayloadHMAC(key, "wecom_external_contact_identity_map", payload)
	if err != nil {
		t.Fatal(err)
	}
	field, err := v1archive.FieldHMAC(key, "wecom_external_contact_identity_map", roots)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: v1deferredidentityhistory.ExternalContactIdentityMapID, SourceOrdinal: 1, SourceKeyHMAC: source, PayloadHMAC: payloadHMAC, FieldHMAC: field, Payload: payload, RedactedFields: roots}
}
