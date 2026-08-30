package compiler

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/dsl"
)

type fakeQuerySet struct {
	calls []string
	fail  error
}

func (set *fakeQuerySet) result(name string, ids ...int64) ([]int64, error) {
	set.calls = append(set.calls, name)
	if set.fail != nil {
		return nil, set.fail
	}
	return ids, nil
}
func (set *fakeQuerySet) Universe(context.Context) ([]int64, error) {
	return set.result("universe", 8, 7, 6, 5, 4, 3, 2, 1, 1)
}
func (set *fakeQuerySet) StageEqual(_ context.Context, value int64) ([]int64, error) {
	if value == 1 {
		return set.result("stage.equal", 5, 2, 1)
	}
	return set.result("stage.equal", 4, 3)
}
func (set *fakeQuerySet) StageAny(context.Context, []int64) ([]int64, error) {
	return set.result("stage.any", 5, 4, 3, 2, 1)
}
func (set *fakeQuerySet) OwnerEqual(context.Context, int64) ([]int64, error) {
	return set.result("owner.equal", 6, 3, 1)
}
func (set *fakeQuerySet) OwnerAny(context.Context, []int64) ([]int64, error) {
	return set.result("owner.any", 7, 6, 4, 3, 1)
}
func (set *fakeQuerySet) ChannelEqual(context.Context, int64) ([]int64, error) {
	return set.result("channel.equal", 8, 4, 1)
}
func (set *fakeQuerySet) ChannelAny(context.Context, []int64) ([]int64, error) {
	return set.result("channel.any", 8, 7, 4, 2, 1)
}
func (set *fakeQuerySet) TagAny(context.Context, []int64) ([]int64, error) {
	return set.result("tag.any", 8, 6, 5, 2)
}
func (set *fakeQuerySet) AddedBefore(context.Context, time.Time) ([]int64, error) {
	return set.result("added.before", 1, 2, 3, 4)
}
func (set *fakeQuerySet) AddedAfter(context.Context, time.Time) ([]int64, error) {
	return set.result("added.after", 5, 6, 7, 8)
}
func (set *fakeQuerySet) LastInteractBefore(context.Context, time.Time) ([]int64, error) {
	return set.result("last.before", 1, 3, 5, 7)
}
func (set *fakeQuerySet) LastInteractAfter(context.Context, time.Time) ([]int64, error) {
	return set.result("last.after", 2, 4, 6, 8)
}
func (set *fakeQuerySet) DeletedEqual(_ context.Context, value bool) ([]int64, error) {
	if value {
		return set.result("deleted.equal", 7, 8)
	}
	return set.result("deleted.equal", 1, 2, 3, 4, 5, 6)
}
func (set *fakeQuerySet) HXCSubscriptionTierEqual(context.Context, string) ([]int64, error) {
	return set.result("hxc.tier", 2, 4, 6)
}
func (set *fakeQuerySet) HXCSubscriptionActiveEqual(context.Context, bool) ([]int64, error) {
	return set.result("hxc.active", 2, 4)
}
func (set *fakeQuerySet) HXCDaysRemainingGTE(context.Context, int64) ([]int64, error) {
	return set.result("hxc.days.gte", 2, 4)
}
func (set *fakeQuerySet) HXCDaysRemainingLTE(context.Context, int64) ([]int64, error) {
	return set.result("hxc.days.lte", 6)
}
func (set *fakeQuerySet) HXCUserMessages7DGTE(context.Context, int64) ([]int64, error) {
	return set.result("hxc.messages7.gte", 2, 4)
}
func (set *fakeQuerySet) HXCUserMessages7DLTE(context.Context, int64) ([]int64, error) {
	return set.result("hxc.messages7.lte", 6)
}
func (set *fakeQuerySet) HXCUserMessages30DGTE(context.Context, int64) ([]int64, error) {
	return set.result("hxc.messages30.gte", 2, 4)
}
func (set *fakeQuerySet) HXCUserMessages30DLTE(context.Context, int64) ([]int64, error) {
	return set.result("hxc.messages30.lte", 6)
}
func (set *fakeQuerySet) HXCLastCapabilityEqual(context.Context, string) ([]int64, error) {
	return set.result("hxc.capability", 2)
}
func (set *fakeQuerySet) HXCBusinessStageEqual(context.Context, string) ([]int64, error) {
	return set.result("hxc.business", 2)
}
func (set *fakeQuerySet) HXCMainLineTypeEqual(context.Context, string) ([]int64, error) {
	return set.result("hxc.mainline", 2)
}
func (set *fakeQuerySet) HXCUserSegmentEqual(context.Context, string) ([]int64, error) {
	return set.result("hxc.segment", 2)
}
func (set *fakeQuerySet) HXCFocusTopicAny(context.Context, []string) ([]int64, error) {
	return set.result("hxc.focus", 2, 4)
}
func (set *fakeQuerySet) HXCPainTagEqual(context.Context, string) ([]int64, error) {
	return set.result("hxc.pain", 2)
}

func TestExecuteHXCFiltersUseMatchedOnlyQueryFamily(t *testing.T) {
	ast, err := dsl.Parse([]byte(`{"and":[{"field":"hxc_subscription_tier","op":"eq","value":"pro"},{"field":"hxc_subscription_active","op":"eq","value":true},{"field":"hxc_focus_topic","op":"has_any","value":["growth"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	program, err := Compile(ast, reference)
	if err != nil {
		t.Fatal(err)
	}
	set := &fakeQuerySet{}
	got, err := Execute(context.Background(), program, set)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{2, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("HXC result = %v, want %v", got, want)
	}
}

func TestExecuteLeafAndCombinationSemanticMatrix(t *testing.T) {
	leaves := []struct {
		definition string
		want       []int64
	}{
		{`{"field":"stage_id","op":"eq","value":1}`, []int64{1, 2, 5}},
		{`{"field":"stage_id","op":"in","value":[1,2]}`, []int64{1, 2, 3, 4, 5}},
		{`{"field":"owner_staff_id","op":"eq","value":1}`, []int64{1, 3, 6}},
		{`{"field":"owner_staff_id","op":"in","value":[1,2]}`, []int64{1, 3, 4, 6, 7}},
		{`{"field":"channel_id","op":"eq","value":1}`, []int64{1, 4, 8}},
		{`{"field":"channel_id","op":"in","value":[1,2]}`, []int64{1, 2, 4, 7, 8}},
		{`{"field":"tag_id","op":"has_any","value":[1,2]}`, []int64{2, 5, 6, 8}},
		{`{"field":"added_at","op":"before","value":"2026-08-01T00:00:00Z"}`, []int64{1, 2, 3, 4}},
		{`{"field":"added_at","op":"after","value":"2026-08-01T00:00:00Z"}`, []int64{5, 6, 7, 8}},
		{`{"field":"last_interact_at","op":"before","value":"2026-08-01T00:00:00Z"}`, []int64{1, 3, 5, 7}},
		{`{"field":"last_interact_at","op":"after","value":"2026-08-01T00:00:00Z"}`, []int64{2, 4, 6, 8}},
		{`{"field":"is_deleted","op":"eq","value":true}`, []int64{7, 8}},
		{`{"field":"is_deleted","op":"eq","value":false}`, []int64{1, 2, 3, 4, 5, 6}},
	}
	cases := make([]struct {
		definition string
		want       []int64
	}, 0, 61)
	cases = append(cases, leaves...)
	for left := 0; left < 6; left++ {
		for right := 6; right < 10; right++ {
			cases = append(cases,
				struct {
					definition string
					want       []int64
				}{fmt.Sprintf(`{"and":[%s,%s]}`, leaves[left].definition, leaves[right].definition), expectedIntersection(leaves[left].want, leaves[right].want)},
				struct {
					definition string
					want       []int64
				}{fmt.Sprintf(`{"or":[%s,%s]}`, leaves[left].definition, leaves[right].definition), expectedUnion(leaves[left].want, leaves[right].want)},
			)
		}
	}
	if len(cases) != 61 {
		t.Fatalf("semantic matrix = %d, want 61", len(cases))
	}
	for index, test := range cases {
		t.Run(fmt.Sprintf("case-%02d", index+1), func(t *testing.T) {
			ast, err := dsl.Parse([]byte(test.definition))
			if err != nil {
				t.Fatal(err)
			}
			program, err := Compile(ast, reference)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Execute(context.Background(), program, &fakeQuerySet{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Execute(%s) = %v, want %v", test.definition, got, test.want)
			}
		})
	}
}

func TestExecuteUniverseComplementOrderAndFailClosed(t *testing.T) {
	set := &fakeQuerySet{}
	program := Program{root: complement{child: leaf{opcode: DeletedEqual, bind: bind{boolean: boolPointer(true)}}}}
	got, err := Execute(context.Background(), program, set)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{1, 2, 3, 4, 5, 6}; !reflect.DeepEqual(got, want) {
		t.Fatalf("complement = %v, want %v", got, want)
	}
	if want := []string{"universe", "deleted.equal"}; !reflect.DeepEqual(set.calls, want) {
		t.Fatalf("calls = %v, want %v", set.calls, want)
	}
	for _, invalid := range []Program{
		{},
		{root: leaf{opcode: Opcode("raw.sql"), bind: bind{}}},
		{root: leaf{opcode: StageEqual, bind: bind{}}},
		{root: intersect{}},
	} {
		if _, err := Execute(context.Background(), invalid, &fakeQuerySet{}); reasonOf(err) != dsl.ReasonCompileUnsafe {
			t.Fatalf("invalid program error = %v", err)
		}
	}
}

func TestExecutePropagatesContextAndStoreErrorsWithoutQueryText(t *testing.T) {
	ast := mustParse(t, `{"field":"stage_id","op":"eq","value":1}`)
	program, err := Compile(ast, reference)
	if err != nil {
		t.Fatal(err)
	}
	storeError := errors.New("database unavailable")
	if _, err := Execute(context.Background(), program, &fakeQuerySet{fail: storeError}); !errors.Is(err, storeError) {
		t.Fatalf("store error = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Execute(cancelled, program, &fakeQuerySet{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("context error = %v", err)
	}
}

func boolPointer(value bool) *bool { return &value }

func expectedIntersection(left, right []int64) []int64 {
	rightSet := make(map[int64]bool, len(right))
	for _, id := range right {
		rightSet[id] = true
	}
	var result []int64
	for _, id := range left {
		if rightSet[id] {
			result = append(result, id)
		}
	}
	return result
}

func expectedUnion(left, right []int64) []int64 {
	seen := make(map[int64]bool, len(left)+len(right))
	for _, ids := range [][]int64{left, right} {
		for _, id := range ids {
			seen[id] = true
		}
	}
	result := make([]int64, 0, len(seen))
	for id := int64(1); id <= 8; id++ {
		if seen[id] {
			result = append(result, id)
		}
	}
	return result
}
