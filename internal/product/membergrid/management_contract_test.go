package membergrid

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestManagementDTOJSONFieldWhitelists(t *testing.T) {
	assertJSONFields(t, SavedView{}, []string{
		"columns", "created_at", "created_by", "name", "service_product_id", "sort", "source_view_id", "state", "updated_at", "version", "view_id",
	})
	assertJSONFields(t, Collaborator{}, []string{
		"collaborator_id", "created_at", "invited_by", "permission", "service_product_id", "staff_id", "updated_at", "version",
	})
	assertJSONFields(t, ShareSettingsResponse{}, []string{
		"collaborator_edit_grants_central_permission", "collaborator_edit_is_local_metadata_only", "collaborators",
		"external_share_enabled", "external_share_supported", "real_external_call_executed", "saved_views", "service_product_id",
	})
	assertJSONFields(t, SavedViewResponse{}, []string{"ok", "view"})
	assertJSONFields(t, DeleteSavedViewResponse{}, []string{"deleted", "ok", "view"})
	assertJSONFields(t, CollaboratorResponse{}, []string{
		"collaborator", "edit_permission_is_local_metadata_only", "grants_central_products_permission", "ok",
	})
	assertJSONFields(t, DeleteCollaboratorResponse{}, []string{
		"collaborator", "deleted", "edit_permission_is_local_metadata_only", "grants_central_products_permission", "ok",
	})

	assertJSONFields(t, CreateSavedViewRequest{}, []string{
		"columns", "expected_version", "name", "sort", "source_view_id", "state",
	})
	assertJSONFields(t, UpdateSavedViewRequest{}, []string{
		"columns", "expected_version", "name", "sort", "state",
	})
	assertJSONFields(t, DeleteVersionedRequest{}, []string{"expected_version"})
	assertJSONFields(t, CreateCollaboratorRequest{}, []string{"expected_version", "permission", "staff_id"})
	assertJSONFields(t, UpdateCollaboratorRequest{}, []string{"expected_version", "permission"})
}

func TestManagementColumnsAreExactlyTheExistingSafeSchemaKeys(t *testing.T) {
	want := make([]string, len(safeColumns))
	for index, column := range safeColumns {
		want[index] = column.Key
	}
	if !reflect.DeepEqual(want, []string{
		"entitlement_id", "product_id", "state", "version", "granted_at", "revoked_at", "display_name", "masked_mobile",
	}) {
		t.Fatalf("unexpected current safe schema=%v", want)
	}
	if !validColumnSelection(want) {
		t.Fatalf("full safe schema was rejected: %v", want)
	}
	for _, forbidden := range []string{
		"customer_id", "unionid", "external_userid", "mobile", "order_id", "provider_receipt", "opaque", "raw_payload", "sql",
	} {
		if validColumnSelection([]string{forbidden}) {
			t.Fatalf("forbidden column accepted=%q", forbidden)
		}
	}
}

func TestManagementResponsesCannotClaimExternalSharingOrCentralPermission(t *testing.T) {
	stamp := time.Date(2026, 8, 22, 5, 6, 7, 0, time.UTC)
	response := struct {
		Settings     ShareSettingsResponse      `json:"settings"`
		Collaborator CollaboratorResponse       `json:"collaborator"`
		Deleted      DeleteCollaboratorResponse `json:"deleted"`
	}{
		Settings: ShareSettingsResponse{
			ServiceProductID: 1, SavedViews: []SavedView{}, Collaborators: []Collaborator{},
			ExternalShareSupported: false, ExternalShareEnabled: false, RealExternalCallExecuted: false,
			CollaboratorEditIsLocalMetadataOnly: true, CollaboratorEditGrantsCentralPermission: false,
		},
		Collaborator: CollaboratorResponse{
			OK: true,
			Collaborator: Collaborator{
				ID: 2, ServiceProductID: 1, StaffID: 3, Permission: CollaboratorPermissionEdit,
				Version: 1, InvitedBy: 4, CreatedAt: stamp, UpdatedAt: stamp,
			},
			EditPermissionIsLocalMetadataOnly: true, GrantsCentralProductsPermission: false,
		},
		Deleted: DeleteCollaboratorResponse{
			OK: true, Deleted: true,
			Collaborator: Collaborator{
				ID: 2, ServiceProductID: 1, StaffID: 3, Permission: CollaboratorPermissionEdit,
				Version: 1, InvitedBy: 4, CreatedAt: stamp, UpdatedAt: stamp,
			},
			EditPermissionIsLocalMetadataOnly: true, GrantsCentralProductsPermission: false,
		},
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ToLower(string(encoded))
	for _, forbidden := range []string{
		"public_token", "public_url", "share_url", "qr_code", "invite_receipt", "provider", "external_userid", "unionid", "customer_id", "raw_payload", "opaque",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("forbidden field/value %q leaked: %s", forbidden, body)
		}
	}
	for _, want := range []string{
		`"external_share_supported":false`, `"external_share_enabled":false`, `"real_external_call_executed":false`,
		`"collaborator_edit_is_local_metadata_only":true`, `"collaborator_edit_grants_central_permission":false`,
		`"edit_permission_is_local_metadata_only":true`, `"grants_central_products_permission":false`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing invariant %s in %s", want, body)
		}
	}
}

func TestManagementEnumsRemainClosed(t *testing.T) {
	for _, state := range []StateFilter{StateActive, StateRevoked, StateAll} {
		if !state.valid() {
			t.Fatalf("valid state rejected=%q", state)
		}
	}
	for _, state := range []StateFilter{"", "pending", "active OR 1=1"} {
		if state.valid() {
			t.Fatalf("invalid state accepted=%q", state)
		}
	}
	if !ViewSortGrantedAtDesc.valid() {
		t.Fatal("closed sort rejected")
	}
	for _, sort := range []ViewSort{"", "granted_at_asc", "sql"} {
		if sort.valid() {
			t.Fatalf("invalid sort accepted=%q", sort)
		}
	}
	for _, permission := range []CollaboratorPermission{CollaboratorPermissionView, CollaboratorPermissionEdit} {
		if !permission.valid() {
			t.Fatalf("valid permission rejected=%q", permission)
		}
	}
	for _, permission := range []CollaboratorPermission{"", "admin", "owner", "invite"} {
		if permission.valid() {
			t.Fatalf("invalid permission accepted=%q", permission)
		}
	}
}
