package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestCustomerMutationRepositoryRejectsTypedNilAndMissingTransactionContext(t *testing.T) {
	command := validCustomerUpdateCommand()

	var nilRepository *CustomerMutationRepository
	if _, err := nilRepository.UpdateCustomer(context.Background(), command); !errors.Is(err, contactapp.ErrCustomerMutationFailed) {
		t.Fatalf("typed nil repository error = %v, want mutation failure", err)
	}

	var nilContext *typedNilMutationContext
	if _, err := NewCustomerMutationRepository().UpdateCustomer(nilContext, command); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("typed nil context error = %v, want transaction requirement", err)
	}
	if _, err := NewCustomerMutationRepository().UpdateCustomer(context.Background(), command); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("missing transaction error = %v, want transaction requirement", err)
	}
}

func TestUpdateCustomerParamsPreserveOmittedAndExplicitNullSemantics(t *testing.T) {
	name := "Ada"
	ignoredAvatar := "https://ignored.example.test/avatar"
	ownerID := int64(11)
	ignoredChannelID := int64(12)
	extra := json.RawMessage(`{"source":"manual"}`)
	params := updateCustomerParams(contactapp.CustomerUpdateCommand{
		ID:        42,
		Name:      &name,
		AvatarURL: contactapp.NullablePatch[string]{Value: &ignoredAvatar},
		Gender:    contactapp.NullablePatch[int16]{Set: true},
		OwnerStaffID: contactapp.NullablePatch[int64]{
			Set: true, Value: &ownerID,
		},
		ChannelID: contactapp.NullablePatch[int64]{Value: &ignoredChannelID},
		Extra:     &extra,
		Actor:     "operator",
	})

	if !params.NameSet || !params.Name.Valid || params.Name.String != name {
		t.Fatalf("name params = %#v, want set %q", params.Name, name)
	}
	if params.AvatarUrlSet || params.AvatarUrl.Valid {
		t.Fatalf("omitted avatar params = %#v, want unset NULL", params.AvatarUrl)
	}
	if !params.GenderSet || params.Gender.Valid {
		t.Fatalf("explicit null gender params = %#v, want set NULL", params.Gender)
	}
	if !params.OwnerStaffIDSet || !params.OwnerStaffID.Valid || params.OwnerStaffID.Int64 != ownerID {
		t.Fatalf("owner params = %#v, want set %d", params.OwnerStaffID, ownerID)
	}
	if params.ChannelIDSet || params.ChannelID.Valid {
		t.Fatalf("omitted channel params = %#v, want unset NULL", params.ChannelID)
	}
	if !params.ExtraSet || string(params.Extra) != string(extra) || params.CustomerID != 42 {
		t.Fatalf("extra/customer params = %#v, want independent extra and id", params)
	}
	extra[2] = 'X'
	if string(params.Extra) != `{"source":"manual"}` {
		t.Fatalf("extra params = %s, want independent copy", params.Extra)
	}
}

func TestSameJSONObjectNeverCollapsesDistinctLargeIntegers(t *testing.T) {
	left := json.RawMessage(`{"value":9007199254740992}`)
	right := json.RawMessage(`{"value":9007199254740993}`)
	if sameJSONObject(left, right) {
		t.Fatal("sameJSONObject() collapsed distinct integers above float64 exact range")
	}
	if !sameJSONObject(
		json.RawMessage(`{"a":1,"b":2}`),
		json.RawMessage(`{"b":2,"a":1}`),
	) {
		t.Fatal("sameJSONObject() rejected equivalent object key order")
	}
}

func TestCustomerUpdateChangesRespectsPatchesAndJSONBSemantics(t *testing.T) {
	avatarURL := "https://example.test/avatar"
	gender := int16(2)
	ownerID := int64(8)
	channelID := int64(9)
	current := contactapp.CustomerRecord{
		ID: 42, Name: "Ada", AvatarURL: &avatarURL, Gender: &gender,
		OwnerStaffID: &ownerID, ChannelID: &channelID, Extra: json.RawMessage(`{"a":1,"b":2}`),
	}
	ignoredChannelID := int64(99)
	semanticallyEqualExtra := json.RawMessage(`{"b":2,"a":1}`)
	noOp := contactapp.CustomerUpdateCommand{
		ID: 42, Name: &current.Name,
		AvatarURL: contactapp.NullablePatch[string]{Set: true, Value: &avatarURL},
		Gender:    contactapp.NullablePatch[int16]{Set: true, Value: &gender},
		OwnerStaffID: contactapp.NullablePatch[int64]{
			Set: true, Value: &ownerID,
		},
		ChannelID: contactapp.NullablePatch[int64]{Value: &ignoredChannelID},
		Extra:     &semanticallyEqualExtra,
		Actor:     "operator",
	}
	if customerUpdateChanges(current, noOp) {
		t.Fatal("equivalent patches must not perform an UPDATE")
	}

	changedOwnerID := ownerID + 1
	noOp.OwnerStaffID.Value = &changedOwnerID
	if !customerUpdateChanges(current, noOp) {
		t.Fatal("changed explicit patch must perform an UPDATE")
	}
}

func TestCustomerMutationRepositoryUpdatesFullCustomerAndSanitizesErrors(t *testing.T) {
	name := "Ada Lovelace"
	gender := int16(2)
	ownerID := int64(8)
	ignoredChannelID := int64(9)
	extra := json.RawMessage(`{"source":"form"}`)
	command := contactapp.CustomerUpdateCommand{
		ID:        42,
		Name:      &name,
		AvatarURL: contactapp.NullablePatch[string]{Set: true},
		Gender:    contactapp.NullablePatch[int16]{Set: true, Value: &gender},
		OwnerStaffID: contactapp.NullablePatch[int64]{
			Set: true, Value: &ownerID,
		},
		ChannelID: contactapp.NullablePatch[int64]{Value: &ignoredChannelID},
		Extra:     &extra,
		Actor:     "operator",
	}

	t.Run("maps every SQL parameter and returns full record", func(t *testing.T) {
		locked := mutationCustomerRow(42, int64PointerValue(3))
		row := mutationCustomerRow(42, int64PointerValue(3))
		tx := newCustomerMutationTx()
		tx.rows["LockActiveCustomerForMutation"] = mutationRowResult{customer: &locked}
		tx.rows["UpdateCustomer"] = mutationRowResult{customer: &row}

		var got contactapp.CustomerProfileMutation
		err := withinCustomerMutationTx(tx, func(ctx context.Context) error {
			var err error
			got, err = NewCustomerMutationRepository().UpdateCustomer(ctx, command)
			return err
		})
		if err != nil {
			t.Fatalf("UpdateCustomer() error = %v", err)
		}
		if !got.StateChange || got.Customer.ID != 42 || got.Customer.StageID == nil || *got.Customer.StageID != 3 || got.Customer.Extra == nil {
			t.Fatalf("UpdateCustomer() record = %#v, want full customer", got)
		}
		if strings.Join(tx.queryOrder, ",") != "LockActiveCustomerForMutation,UpdateCustomer" {
			t.Fatalf("query order = %v, want lock then update", tx.queryOrder)
		}
		args := tx.queryArgs["UpdateCustomer"]
		if len(args) != 13 {
			t.Fatalf("UpdateCustomer argument count = %d, want 13", len(args))
		}
		assertBooleanArgument(t, args[0], true, "name_set")
		assertTextArgument(t, args[1], name, true, "name")
		assertBooleanArgument(t, args[2], true, "avatar_url_set")
		assertTextArgument(t, args[3], "", false, "avatar_url explicit null")
		assertBooleanArgument(t, args[4], true, "gender_set")
		assertInt16Argument(t, args[5], gender, true, "gender")
		assertBooleanArgument(t, args[6], true, "owner_staff_id_set")
		assertInt64Argument(t, args[7], ownerID, true, "owner_staff_id")
		assertBooleanArgument(t, args[8], false, "channel_id_set")
		assertInt64Argument(t, args[9], 0, false, "channel_id omitted")
		if value, ok := args[10].(bool); !ok || !value {
			t.Fatalf("extra_set = %#v, want true", args[10])
		}
		if value, ok := args[11].([]byte); !ok || string(value) != string(extra) {
			t.Fatalf("extra = %#v, want %s", args[11], extra)
		}
		if value, ok := args[12].(int64); !ok || value != 42 {
			t.Fatalf("customer_id = %#v, want 42", args[12])
		}
	})

	t.Run("full no-op returns locked record without update", func(t *testing.T) {
		locked := mutationCustomerRow(42, nil)
		unchangedName := locked.Name
		tx := newCustomerMutationTx()
		tx.rows["LockActiveCustomerForMutation"] = mutationRowResult{customer: &locked}

		var got contactapp.CustomerProfileMutation
		err := withinCustomerMutationTx(tx, func(ctx context.Context) error {
			var err error
			got, err = NewCustomerMutationRepository().UpdateCustomer(ctx, contactapp.CustomerUpdateCommand{
				ID: 42, Name: &unchangedName, Actor: "operator",
			})
			return err
		})
		if err != nil {
			t.Fatalf("UpdateCustomer() error = %v", err)
		}
		if got.StateChange || got.Customer.ID != 42 || strings.Join(tx.queryOrder, ",") != "LockActiveCustomerForMutation" {
			t.Fatalf("no-op mutation/order = %#v/%v, want unchanged lock-only result", got, tx.queryOrder)
		}
	})

	for _, testCase := range []struct {
		name string
		err  error
		want error
	}{
		{name: "missing customer", err: pgx.ErrNoRows, want: contactapp.ErrCustomerNotFound},
		{name: "stage foreign key", err: &pgconn.PgError{Code: "23503", ConstraintName: "customers_stage_id_fkey", Message: "sensitive database detail"}, want: contactport.ErrStageNotFound},
		{name: "unique conflict", err: &pgconn.PgError{Code: "23505", Message: "sensitive database detail"}, want: contactapp.ErrCustomerConflict},
		{name: "unknown database error", err: errors.New("sensitive database detail"), want: contactapp.ErrCustomerMutationFailed},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			locked := mutationCustomerRow(42, nil)
			tx := newCustomerMutationTx()
			tx.rows["LockActiveCustomerForMutation"] = mutationRowResult{customer: &locked}
			tx.rows["UpdateCustomer"] = mutationRowResult{err: testCase.err}
			err := withinCustomerMutationTx(tx, func(ctx context.Context) error {
				_, err := NewCustomerMutationRepository().UpdateCustomer(ctx, command)
				return err
			})
			if !errors.Is(err, testCase.want) {
				t.Fatalf("UpdateCustomer() error = %v, want %v", err, testCase.want)
			}
			if strings.Contains(err.Error(), "sensitive database detail") {
				t.Fatalf("UpdateCustomer() leaked database detail: %v", err)
			}
		})
	}
}

func TestCustomerMutationRepositorySetsStageWithLockAndIdempotence(t *testing.T) {
	stageID := int64(3)
	command := contactapp.CustomerStageCommand{ID: 42, StageID: &stageID, Actor: "operator"}

	t.Run("same stage is locked but not updated", func(t *testing.T) {
		locked := mutationCustomerRow(42, &stageID)
		tx := newCustomerMutationTx()
		tx.rows["LockActiveCustomerForMutation"] = mutationRowResult{customer: &locked}

		var got contactapp.CustomerStageMutation
		err := withinCustomerMutationTx(tx, func(ctx context.Context) error {
			var err error
			got, err = NewCustomerMutationRepository().SetCustomerStage(ctx, command)
			return err
		})
		if err != nil {
			t.Fatalf("SetCustomerStage() error = %v", err)
		}
		if got.StateChange || got.PreviousID == nil || *got.PreviousID != stageID || got.Customer.ID != 42 {
			t.Fatalf("same stage mutation = %#v, want idempotent result", got)
		}
		if strings.Join(tx.queryOrder, ",") != "LockActiveCustomerForMutation" {
			t.Fatalf("query order = %v, want lock only", tx.queryOrder)
		}
	})

	t.Run("different stage returns previous and changes once", func(t *testing.T) {
		previousID := int64(2)
		locked := mutationCustomerRow(42, &previousID)
		updated := mutationCustomerRow(42, &stageID)
		tx := newCustomerMutationTx()
		tx.rows["LockActiveCustomerForMutation"] = mutationRowResult{customer: &locked}
		tx.rows["SetCustomerStage"] = mutationRowResult{customer: &updated}

		var got contactapp.CustomerStageMutation
		err := withinCustomerMutationTx(tx, func(ctx context.Context) error {
			var err error
			got, err = NewCustomerMutationRepository().SetCustomerStage(ctx, command)
			return err
		})
		if err != nil {
			t.Fatalf("SetCustomerStage() error = %v", err)
		}
		if !got.StateChange || got.PreviousID == nil || *got.PreviousID != previousID || got.Customer.StageID == nil || *got.Customer.StageID != stageID {
			t.Fatalf("stage mutation = %#v, want changed full record", got)
		}
		if strings.Join(tx.queryOrder, ",") != "LockActiveCustomerForMutation,SetCustomerStage" {
			t.Fatalf("query order = %v, want lock then update", tx.queryOrder)
		}
		args := tx.queryArgs["SetCustomerStage"]
		if len(args) != 2 {
			t.Fatalf("SetCustomerStage arguments = %#v, want stage and customer", args)
		}
		assertInt64Argument(t, args[0], stageID, true, "stage_id")
		if value, ok := args[1].(int64); !ok || value != 42 {
			t.Fatalf("customer_id = %#v, want 42", args[1])
		}
	})

	t.Run("missing customer and missing stage are frozen errors", func(t *testing.T) {
		tests := []struct {
			name  string
			lock  mutationRowResult
			stage mutationRowResult
			want  error
		}{
			{name: "missing customer", lock: mutationRowResult{err: pgx.ErrNoRows}, want: contactapp.ErrCustomerNotFound},
			{
				name:  "missing stage foreign key",
				lock:  mutationRowResult{customer: customerPointer(mutationCustomerRow(42, nil))},
				stage: mutationRowResult{err: &pgconn.PgError{Code: "23503", ConstraintName: "customers_stage_id_fkey", Message: "hidden"}},
				want:  contactport.ErrStageNotFound,
			},
		}
		for _, testCase := range tests {
			t.Run(testCase.name, func(t *testing.T) {
				tx := newCustomerMutationTx()
				tx.rows["LockActiveCustomerForMutation"] = testCase.lock
				tx.rows["SetCustomerStage"] = testCase.stage
				err := withinCustomerMutationTx(tx, func(ctx context.Context) error {
					_, err := NewCustomerMutationRepository().SetCustomerStage(ctx, command)
					return err
				})
				if !errors.Is(err, testCase.want) {
					t.Fatalf("SetCustomerStage() error = %v, want %v", err, testCase.want)
				}
			})
		}
	})
}

func TestCustomerMutationRepositoryAddsAndRemovesTagsIdempotently(t *testing.T) {
	command := contactapp.CustomerTagCommand{ID: 42, TagID: 7, Actor: "operator"}

	for _, testCase := range []struct {
		name  string
		add   bool
		rows  int64
		want  bool
		query string
	}{
		{name: "adds new tag", add: true, rows: 1, want: true, query: "AddCustomerTag"},
		{name: "existing tag stays idempotent", add: true, rows: 0, want: false, query: "AddCustomerTag"},
		{name: "removes applied tag", add: false, rows: 1, want: true, query: "RemoveCustomerTag"},
		{name: "removed tag stays idempotent", add: false, rows: 0, want: false, query: "RemoveCustomerTag"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			locked := mutationCustomerRow(42, nil)
			tagID := int64(7)
			tx := newCustomerMutationTx()
			tx.rows["LockActiveCustomerForMutation"] = mutationRowResult{customer: &locked}
			tx.rows["GetCustomerTag"] = mutationRowResult{int64Value: &tagID}
			tx.execRows[testCase.query] = testCase.rows

			var changed bool
			err := withinCustomerMutationTx(tx, func(ctx context.Context) error {
				var err error
				if testCase.add {
					changed, err = NewCustomerMutationRepository().AddCustomerTag(ctx, command)
				} else {
					changed, err = NewCustomerMutationRepository().RemoveCustomerTag(ctx, command)
				}
				return err
			})
			if err != nil || changed != testCase.want {
				t.Fatalf("tag mutation changed/error = %t/%v, want %t/nil", changed, err, testCase.want)
			}
			if strings.Join(tx.queryOrder, ",") != "LockActiveCustomerForMutation,GetCustomerTag" {
				t.Fatalf("query order = %v, want customer lock then tag validation", tx.queryOrder)
			}
			args := tx.execArgs[testCase.query]
			if testCase.add {
				if len(args) != 3 || args[0] != int64(42) || args[1] != int64(7) || args[2] != "operator" {
					t.Fatalf("AddCustomerTag arguments = %#v", args)
				}
			} else if len(args) != 2 || args[0] != int64(42) || args[1] != int64(7) {
				t.Fatalf("RemoveCustomerTag arguments = %#v", args)
			}
		})
	}

	t.Run("missing tag and concurrent tag FK use tag sentinel", func(t *testing.T) {
		for _, testCase := range []struct {
			name      string
			tagResult mutationRowResult
			execError error
			want      error
		}{
			{name: "catalog tag missing", tagResult: mutationRowResult{err: pgx.ErrNoRows}, want: contactapp.ErrCustomerTagNotFound},
			{name: "tag deleted after validation", tagResult: mutationRowResult{int64Value: int64PointerValue(7)}, execError: &pgconn.PgError{Code: "23503", ConstraintName: "customer_tags_tag_id_fkey"}, want: contactapp.ErrCustomerTagNotFound},
		} {
			t.Run(testCase.name, func(t *testing.T) {
				locked := mutationCustomerRow(42, nil)
				tx := newCustomerMutationTx()
				tx.rows["LockActiveCustomerForMutation"] = mutationRowResult{customer: &locked}
				tx.rows["GetCustomerTag"] = testCase.tagResult
				tx.execErr["AddCustomerTag"] = testCase.execError
				err := withinCustomerMutationTx(tx, func(ctx context.Context) error {
					_, err := NewCustomerMutationRepository().AddCustomerTag(ctx, command)
					return err
				})
				if !errors.Is(err, testCase.want) {
					t.Fatalf("AddCustomerTag() error = %v, want %v", err, testCase.want)
				}
			})
		}
	})
}

func TestCustomerMutationRepositoryAppendsCurrentTimelineEvent(t *testing.T) {
	occurredAt := time.Date(2026, time.August, 12, 8, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	payload := json.RawMessage(`{"stage_id":3}`)
	command := contactapp.CustomerEventAppend{
		CustomerID: 42,
		EventType:  "customer.stage_changed",
		Payload:    payload,
		Actor:      "operator",
		OccurredAt: occurredAt,
	}

	t.Run("maps append parameters and id", func(t *testing.T) {
		id := int64(99)
		tx := newCustomerMutationTx()
		tx.rows["AppendCustomerEvent"] = mutationRowResult{int64Value: &id}

		var got contactport.EventID
		err := withinCustomerMutationTx(tx, func(ctx context.Context) error {
			var err error
			got, err = NewCustomerMutationRepository().AppendCustomerEvent(ctx, command)
			return err
		})
		if err != nil || got != 99 {
			t.Fatalf("AppendCustomerEvent() id/error = %d/%v, want 99/nil", got, err)
		}
		args := tx.queryArgs["AppendCustomerEvent"]
		if len(args) != 5 || args[0] != int64(42) || args[1] != command.EventType || args[3] != "operator" {
			t.Fatalf("AppendCustomerEvent arguments = %#v", args)
		}
		if storedPayload, ok := args[2].([]byte); !ok || string(storedPayload) != string(payload) {
			t.Fatalf("event payload = %#v, want %s", args[2], payload)
		}
		payload[2] = 'X'
		if storedPayload := args[2].([]byte); string(storedPayload) != `{"stage_id":3}` {
			t.Fatalf("event payload = %s, want independent copy", storedPayload)
		}
		occurredArgument, ok := args[4].(pgtype.Timestamptz)
		if !ok || !occurredArgument.Valid || !occurredArgument.Time.Equal(occurredAt.UTC()) {
			t.Fatalf("occurred_at = %#v, want UTC %s", args[4], occurredAt.UTC())
		}
	})

	for _, testCase := range []struct {
		name string
		err  error
		want error
	}{
		{name: "missing customer", err: pgx.ErrNoRows, want: contactapp.ErrCustomerNotFound},
		{name: "customer FK", err: &pgconn.PgError{Code: "23503", ConstraintName: "customer_events_customer_id_fkey"}, want: contactapp.ErrCustomerNotFound},
		{name: "unknown error", err: errors.New("database internal state"), want: contactapp.ErrCustomerMutationFailed},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			tx := newCustomerMutationTx()
			tx.rows["AppendCustomerEvent"] = mutationRowResult{err: testCase.err}
			err := withinCustomerMutationTx(tx, func(ctx context.Context) error {
				_, err := NewCustomerMutationRepository().AppendCustomerEvent(ctx, command)
				return err
			})
			if !errors.Is(err, testCase.want) {
				t.Fatalf("AppendCustomerEvent() error = %v, want %v", err, testCase.want)
			}
		})
	}
}

func validCustomerUpdateCommand() contactapp.CustomerUpdateCommand {
	name := "Ada"
	return contactapp.CustomerUpdateCommand{ID: 42, Name: &name, Actor: "operator"}
}

func mutationCustomerRow(id int64, stageID *int64) contactdb.Customer {
	at := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	row := contactdb.Customer{
		ID:        id,
		Name:      "Ada",
		Extra:     []byte(`{"source":"test"}`),
		CreatedAt: pgtype.Timestamptz{Time: at.Add(-time.Hour), Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: at, Valid: true},
	}
	if stageID != nil {
		row.StageID = pgtype.Int8{Int64: *stageID, Valid: true}
	}
	return row
}

func customerPointer(row contactdb.Customer) *contactdb.Customer { return &row }

func int64PointerValue(value int64) *int64 { return &value }

func assertBooleanArgument(t *testing.T, value any, want bool, label string) {
	t.Helper()
	got, ok := value.(bool)
	if !ok || got != want {
		t.Fatalf("%s = %#v, want bool %t", label, value, want)
	}
}

func assertTextArgument(t *testing.T, value any, want string, valid bool, label string) {
	t.Helper()
	got, ok := value.(pgtype.Text)
	if !ok || got.String != want || got.Valid != valid {
		t.Fatalf("%s = %#v, want text %q valid=%t", label, value, want, valid)
	}
}

func assertInt16Argument(t *testing.T, value any, want int16, valid bool, label string) {
	t.Helper()
	got, ok := value.(pgtype.Int2)
	if !ok || got.Int16 != want || got.Valid != valid {
		t.Fatalf("%s = %#v, want int16 %d valid=%t", label, value, want, valid)
	}
}

func assertInt64Argument(t *testing.T, value any, want int64, valid bool, label string) {
	t.Helper()
	got, ok := value.(pgtype.Int8)
	if !ok || got.Int64 != want || got.Valid != valid {
		t.Fatalf("%s = %#v, want int64 %d valid=%t", label, value, want, valid)
	}
}

func withinCustomerMutationTx(tx *customerMutationTx, callback func(context.Context) error) error {
	uow := platformstore.NewUnitOfWork(&customerMutationBeginner{tx: tx})
	return uow.Within(context.Background(), callback)
}

type typedNilMutationContext struct{}

func (*typedNilMutationContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*typedNilMutationContext) Done() <-chan struct{}       { return nil }
func (*typedNilMutationContext) Err() error                  { return nil }
func (*typedNilMutationContext) Value(any) any               { return nil }

type customerMutationBeginner struct {
	tx *customerMutationTx
}

func (beginner *customerMutationBeginner) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return beginner.tx, nil
}

type customerMutationTx struct {
	rows       map[string]mutationRowResult
	execRows   map[string]int64
	execErr    map[string]error
	queryArgs  map[string][]any
	execArgs   map[string][]any
	queryOrder []string
	commits    int
	rollbacks  int
}

func newCustomerMutationTx() *customerMutationTx {
	return &customerMutationTx{
		rows:      make(map[string]mutationRowResult),
		execRows:  make(map[string]int64),
		execErr:   make(map[string]error),
		queryArgs: make(map[string][]any),
		execArgs:  make(map[string][]any),
	}
}

func (*customerMutationTx) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("nested transaction is not implemented")
}
func (tx *customerMutationTx) Commit(context.Context) error {
	tx.commits++
	return nil
}
func (tx *customerMutationTx) Rollback(context.Context) error {
	tx.rollbacks++
	return nil
}
func (*customerMutationTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("bulk operation is not implemented")
}
func (*customerMutationTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (*customerMutationTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (*customerMutationTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("prepare is not implemented")
}
func (tx *customerMutationTx) Exec(_ context.Context, statement string, args ...any) (pgconn.CommandTag, error) {
	name := customerMutationQueryName(statement)
	tx.execArgs[name] = append([]any(nil), args...)
	if err := tx.execErr[name]; err != nil {
		return pgconn.CommandTag{}, err
	}
	return pgconn.NewCommandTag(strings.Join([]string{"UP", "DATE", " ", int64String(tx.execRows[name])}, "")), nil
}
func (*customerMutationTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("query is not implemented")
}
func (tx *customerMutationTx) QueryRow(_ context.Context, statement string, args ...any) pgx.Row {
	name := customerMutationQueryName(statement)
	tx.queryOrder = append(tx.queryOrder, name)
	tx.queryArgs[name] = append([]any(nil), args...)
	result, ok := tx.rows[name]
	if !ok {
		return mutationRowResult{err: errors.New("unexpected query: " + name)}
	}
	return result
}
func (*customerMutationTx) Conn() *pgx.Conn { return nil }

type mutationRowResult struct {
	customer   *contactdb.Customer
	int64Value *int64
	err        error
}

func (row mutationRowResult) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if row.customer != nil {
		if len(dest) != 13 {
			return errors.New("unexpected customer scan destination count")
		}
		customer := *row.customer
		*dest[0].(*int64) = customer.ID
		*dest[1].(*string) = customer.Name
		*dest[2].(*pgtype.Text) = customer.AvatarUrl
		*dest[3].(*pgtype.Int2) = customer.Gender
		*dest[4].(*pgtype.Int8) = customer.StageID
		*dest[5].(*pgtype.Int8) = customer.OwnerStaffID
		*dest[6].(*pgtype.Int8) = customer.ChannelID
		*dest[7].(*pgtype.Timestamptz) = customer.AddedAt
		*dest[8].(*pgtype.Timestamptz) = customer.LastInteractAt
		*dest[9].(*bool) = customer.IsDeleted
		*dest[10].(*[]byte) = append([]byte(nil), customer.Extra...)
		*dest[11].(*pgtype.Timestamptz) = customer.CreatedAt
		*dest[12].(*pgtype.Timestamptz) = customer.UpdatedAt
		return nil
	}
	if row.int64Value != nil && len(dest) == 1 {
		*dest[0].(*int64) = *row.int64Value
		return nil
	}
	return errors.New("missing row fixture")
}

func customerMutationQueryName(statement string) string {
	for _, name := range []string{
		"UpdateCustomer",
		"LockActiveCustomerForMutation",
		"SetCustomerStage",
		"GetCustomerTag",
		"AddCustomerTag",
		"RemoveCustomerTag",
		"AppendCustomerEvent",
	} {
		if strings.Contains(statement, "-- name: "+name+" ") {
			return name
		}
	}
	return "unexpected"
}

func int64String(value int64) string {
	if value == 0 {
		return "0"
	}
	if value == 1 {
		return "1"
	}
	return "2"
}
