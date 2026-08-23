package identity_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var errTrustedWeComReaderRollback = errors.New("rollback trusted WeCom reader fixture")

func TestTrustedWeComReaderRequiresTransactionAndOmitsMissingAmbiguousAndUnverifiedValues(t *testing.T) {
	pool := openIdentityPool(t)
	repository := identitystore.NewRepository()
	ctx := context.Background()
	overMaximum := make([]contactport.CustomerID, identityport.MaximumTrustedWeComIdentityCustomerIDs+1)
	for index := range overMaximum {
		overMaximum[index] = contactport.CustomerID(index + 1)
	}
	if _, err := repository.ListPrimaryWeComExternalUserIDs(ctx, overMaximum); !errors.Is(err, identityapp.ErrInvalidIdentity) {
		t.Fatalf("over-maximum read error=%v, want ErrInvalidIdentity before transaction lookup", err)
	}

	if _, err := repository.ListPrimaryWeComExternalUserIDs(ctx, []contactport.CustomerID{1}); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("plain-context read error=%v, want ErrTransactionRequired", err)
	}

	err := platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		tx, txErr := platformstore.TxFromContext(txCtx)
		if txErr != nil {
			return txErr
		}
		customerIDs := make([]contactport.CustomerID, 4)
		for index := range customerIDs {
			if txErr = tx.QueryRow(txCtx, `INSERT INTO customers(name) VALUES($1) RETURNING id`,
				fmt.Sprintf("trusted-wecom-%d-%d", time.Now().UnixNano(), index)).Scan(&customerIDs[index]); txErr != nil {
				return txErr
			}
		}
		for _, fixture := range []struct {
			customerID              contactport.CustomerID
			scope, value, assurance string
		}{
			{customerIDs[1], "corp-a", "wm_unique", "verified"},
			{customerIDs[1], "corp-b", "wm_unique", "verified"},
			{customerIDs[2], "corp-a", "wm_first", "verified"},
			{customerIDs[2], "corp-b", "wm_second", "verified"},
			{customerIDs[3], "corp-a", "wm_declared", "declared"},
		} {
			if txErr = insertTrustedWeComIdentity(txCtx, tx, fixture.customerID, fixture.scope, fixture.value, fixture.assurance); txErr != nil {
				return txErr
			}
		}

		items, readErr := repository.ListPrimaryWeComExternalUserIDs(txCtx, []contactport.CustomerID{
			customerIDs[3], customerIDs[1], customerIDs[0], customerIDs[2],
		})
		want := []identityport.TrustedWeComExternalIdentity{{
			CustomerID: customerIDs[1], ExternalUserID: "wm_unique",
		}}
		if readErr != nil || !reflect.DeepEqual(items, want) {
			return fmt.Errorf("trusted identities=%#v err=%v, want %#v", items, readErr, want)
		}
		return errTrustedWeComReaderRollback
	})
	if !errors.Is(err, errTrustedWeComReaderRollback) {
		t.Fatalf("transactional read error=%v, want rollback sentinel", err)
	}
}

func insertTrustedWeComIdentity(
	ctx context.Context,
	tx pgx.Tx,
	customerID contactport.CustomerID,
	scope, value, assurance string,
) error {
	_, err := tx.Exec(ctx, `
INSERT INTO identities
  (customer_id, kind, scope, normalized_value, normalizer_version, assurance, source,
   review_fingerprint, fingerprint_key_version, bound_at)
VALUES ($1::bigint, 'wecom_external_userid', $2::text, $3::text, 1, $4::text,
        'trusted-wecom-reader-acceptance', decode('00112233445566778899aabbccddeeff', 'hex'), 1, now())`,
		customerID, scope, value, assurance)
	return err
}
