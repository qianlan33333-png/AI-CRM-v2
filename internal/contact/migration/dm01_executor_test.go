package migration

import (
	"context"
	"errors"
	"testing"
)

type corpValidationSnapshot struct {
	SourceSnapshot
}

func (snapshot corpValidationSnapshot) EachExternalIdentityMap(_ context.Context, _ SourceUpperBound, emit func(ExternalIdentityMapRow) error) error {
	return emit(ExternalIdentityMapRow{ID: 1, CorpID: "wrong", Payload: []byte(`{}`)})
}
func (snapshot corpValidationSnapshot) EachResolutionQueue(_ context.Context, _ SourceUpperBound, emit func(ResolutionQueueRow) error) error {
	return emit(ResolutionQueueRow{ID: 1, CorpID: "wrong", Payload: []byte(`{}`)})
}
func (snapshot corpValidationSnapshot) EachDirectoryMember(_ context.Context, _ SourceUpperBound, emit func(DirectoryMemberRow) error) error {
	return emit(DirectoryMemberRow{ID: 1, CorpID: "wrong", Payload: []byte(`{}`)})
}
func (snapshot corpValidationSnapshot) EachFollowUser(_ context.Context, _ SourceUpperBound, emit func(FollowUserRow) error) error {
	return emit(FollowUserRow{ID: 1, CorpID: "wrong", Payload: []byte(`{}`)})
}

func TestAllCorpBearingSourceScansRejectManifestMismatch(t *testing.T) {
	for _, table := range []string{"wecom_external_contact_identity_map", "crm_user_identity_resolution_queue", "admin_wecom_directory_members", "wecom_external_contact_follow_users"} {
		t.Run(table, func(t *testing.T) {
			snapshot := corpValidationSnapshot{}
			for _, scan := range allScans(snapshot, "corp-1") {
				if scan.table != table {
					continue
				}
				if err := scan.each(context.Background(), SourceUpperBound{}, func(string, []byte) error { return nil }); !errors.Is(err, ErrInvalidExecutor) {
					t.Fatalf("corp mismatch error = %v", err)
				}
				return
			}
			t.Fatal("corp-bearing scan missing")
		})
	}
}

func TestTableTrackerRejectsEmptyOrNonCanonicalLineageKey(t *testing.T) {
	tracker := newTableTracker("crm_user_identity", SourceUpperBound{}, make([]byte, 32))
	for _, key := range []string{"", " union", "union "} {
		if _, err := tracker.add(key, []byte(`{}`)); err == nil {
			t.Fatalf("source key %q accepted", key)
		}
	}
}

func TestActiveSourceRowsRejectEmptyIdentityKeys(t *testing.T) {
	if validCustomerSourceRow(CustomerIdentityRow{}) {
		t.Fatal("empty customer unionid accepted")
	}
	if validExternalIdentitySourceRow(ExternalIdentityMapRow{ID: 1, CorpID: "corp"}) {
		t.Fatal("empty external identity keys accepted")
	}
	if !validCustomerSourceRow(CustomerIdentityRow{UnionID: "union"}) || !validExternalIdentitySourceRow(ExternalIdentityMapRow{ID: 1, ExternalUserID: "external", UnionID: "union", CorpID: "corp"}) {
		t.Fatal("canonical identity keys rejected")
	}
}
