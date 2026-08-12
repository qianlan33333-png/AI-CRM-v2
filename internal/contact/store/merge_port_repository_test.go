package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

func TestMergePortRepositoryRejectsInvalidCommandsBeforeTransactionAccess(t *testing.T) {
	zeroID := int64(0)
	invalidUTF8 := string([]byte{0xff})
	repository := NewMergePortRepository()

	for name, command := range map[string]contactport.CreateForIdentityCommand{
		"invalid utf8 name": {Name: invalidUTF8, Actor: "acceptance"},
		"long name":         {Name: strings.Repeat("n", maximumIdentityCustomerName+1), Actor: "acceptance"},
		"blank actor":       {Name: "customer", Actor: ""},
		"zero owner":        {Name: "customer", Actor: "acceptance", OwnerStaffID: &zeroID},
	} {
		t.Run("create/"+name, func(t *testing.T) {
			if _, err := repository.CreateForIdentity(context.Background(), command); !errors.Is(err, contactport.ErrInvalidMergeCommand) {
				t.Fatalf("CreateForIdentity(%+v) error=%v", command, err)
			}
		})
	}
	if _, err := repository.CreateForIdentity(context.Background(), contactport.CreateForIdentityCommand{
		Name: "", Actor: "acceptance",
	}); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("empty channel-neutral name should reach transaction boundary, error=%v", err)
	}

	for name, command := range map[string]contactport.MergeCustomersCommand{
		"missing primary":  {MergedID: 2, Actor: "acceptance", Reason: "reason"},
		"missing merged":   {PrimaryID: 1, Actor: "acceptance", Reason: "reason"},
		"self merge":       {PrimaryID: 1, MergedID: 1, Actor: "acceptance", Reason: "reason"},
		"blank actor":      {PrimaryID: 1, MergedID: 2, Reason: "reason"},
		"untrimmed reason": {PrimaryID: 1, MergedID: 2, Actor: "acceptance", Reason: " reason"},
		"long reason":      {PrimaryID: 1, MergedID: 2, Actor: "acceptance", Reason: strings.Repeat("r", maximumMergeReason+1)},
	} {
		t.Run("merge/"+name, func(t *testing.T) {
			if err := repository.MergeCustomers(context.Background(), command); !errors.Is(err, contactport.ErrInvalidMergeCommand) {
				t.Fatalf("MergeCustomers(%+v) error=%v", command, err)
			}
		})
	}

	var nilRepository *MergePortRepository
	if _, err := nilRepository.CreateForIdentity(context.Background(), contactport.CreateForIdentityCommand{
		Actor: "acceptance",
	}); !errors.Is(err, contactport.ErrInvalidMergeCommand) {
		t.Fatalf("nil repository error=%v", err)
	}
}

func TestMapMergePortDatabaseErrorUsesStableSentinels(t *testing.T) {
	for _, testCase := range []struct {
		name string
		err  error
		want error
	}{
		{name: "non PostgreSQL", err: errors.New("sensitive driver detail"), want: contactport.ErrMergeStoreFailed},
		{name: "foreign key", err: &pgconn.PgError{Code: "23503", Message: "sensitive"}, want: contactport.ErrMergeCustomerNotFound},
		{name: "unique", err: &pgconn.PgError{Code: "23505", Message: "sensitive"}, want: contactport.ErrMergeConflict},
		{name: "check", err: &pgconn.PgError{Code: "23514", Message: "sensitive"}, want: contactport.ErrMergeConflict},
		{name: "too long", err: &pgconn.PgError{Code: "22001", Message: "sensitive"}, want: contactport.ErrInvalidMergeCommand},
		{name: "numeric range", err: &pgconn.PgError{Code: "22003", Message: "sensitive"}, want: contactport.ErrInvalidMergeCommand},
		{name: "invalid text", err: &pgconn.PgError{Code: "22P02", Message: "sensitive"}, want: contactport.ErrInvalidMergeCommand},
		{name: "unknown", err: &pgconn.PgError{Code: "XX000", Message: "sensitive"}, want: contactport.ErrMergeStoreFailed},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			mapped := mapMergePortDatabaseError(testCase.err)
			if !errors.Is(mapped, testCase.want) || mapped.Error() != testCase.want.Error() || strings.Contains(mapped.Error(), "sensitive") {
				t.Fatalf("mapped error=%q want stable %q", mapped, testCase.want)
			}
		})
	}
}

func TestMapMergePortDatabaseErrorPreservesRetryableCauseWithoutLeakingIt(t *testing.T) {
	for _, code := range []string{"40001", "40P01"} {
		t.Run(code, func(t *testing.T) {
			cause := &pgconn.PgError{Code: code, Message: "sensitive database detail"}
			mapped := mapMergePortDatabaseError(cause)

			if !errors.Is(mapped, contactport.ErrMergeStoreFailed) {
				t.Fatalf("mapped error=%v, want stable store sentinel", mapped)
			}
			var databaseError *pgconn.PgError
			if !errors.As(mapped, &databaseError) || databaseError != cause {
				t.Fatalf("mapped cause=%#v, want original retryable PostgreSQL error", databaseError)
			}
			if mapped.Error() != contactport.ErrMergeStoreFailed.Error() || strings.Contains(mapped.Error(), cause.Message) {
				t.Fatalf("mapped error leaked database detail: %q", mapped.Error())
			}
		})
	}
}
