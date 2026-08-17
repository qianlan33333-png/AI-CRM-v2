package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

type facetTestUOW struct {
	calls int
	err   error
}

type facetTestStore struct {
	rows       []FacetRow
	err        error
	readCalls  int
	writeCalls int
}

func (uow *facetTestUOW) Within(ctx context.Context, operation func(context.Context) error) error {
	uow.calls++
	if uow.err != nil {
		return uow.err
	}
	return operation(ctx)
}

func (store *facetTestStore) ListFacetRows(context.Context) ([]FacetRow, error) {
	store.readCalls++
	if store.err != nil {
		return nil, store.err
	}
	return append([]FacetRow(nil), store.rows...), nil
}

func (store *facetTestStore) Reserve(context.Context, Reservation) (Receipt, bool, error) {
	store.writeCalls++
	return Receipt{}, false, errors.New("unexpected reserve")
}

func (store *facetTestStore) Create(context.Context, CreateInput) (mediaport.Image, error) {
	store.writeCalls++
	return mediaport.Image{}, errors.New("unexpected create")
}

func (store *facetTestStore) Complete(context.Context, int64, json.RawMessage, time.Time) (Receipt, error) {
	store.writeCalls++
	return Receipt{}, errors.New("unexpected complete")
}

func TestImageFacetsProjectsTrimmedCaseSensitiveSortedValues(t *testing.T) {
	store := &facetTestStore{rows: []FacetRow{
		{Category: " beta ", Tags: " beta,Alpha, beta ,,中文，逗号"},
		{Category: "Alpha", Tags: "alpha, Alpha"},
		{Category: "Alpha ", Tags: "  "},
		{Category: "Beta", Tags: ",,"},
		{Category: "\u2003", Tags: ""},
	}}
	uow := &facetTestUOW{}
	service := NewService(uow, store, nil)

	result, err := service.Facets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Alpha", "Beta", "beta"}; !reflect.DeepEqual(result.Categories, want) {
		t.Fatalf("categories=%q want=%q", result.Categories, want)
	}
	if want := []string{"Alpha", "alpha", "beta", "中文，逗号"}; !reflect.DeepEqual(result.Tags, want) {
		t.Fatalf("tags=%q want=%q", result.Tags, want)
	}
	if uow.calls != 1 || store.readCalls != 1 || store.writeCalls != 0 {
		t.Fatalf("uow=%d reads=%d writes=%d", uow.calls, store.readCalls, store.writeCalls)
	}
}

func TestImageFacetsPreservesLongCategoriesAndTruncatesTagsByCodePoint(t *testing.T) {
	category80 := strings.Repeat("a", 80)
	category81 := strings.Repeat("b", 81)
	category200 := strings.Repeat("c", 200)
	longTag := strings.Repeat("界", 63) + "🙂" + "尾"
	expectedTag := strings.Repeat("界", 63) + "🙂"
	store := &facetTestStore{rows: []FacetRow{
		{Category: " " + category200 + " ", Tags: longTag},
		{Category: category81, Tags: ""},
		{Category: category80, Tags: ""},
	}}
	service := NewService(&facetTestUOW{}, store, nil)

	result, err := service.Facets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{category80, category81, category200}; !reflect.DeepEqual(result.Categories, want) {
		lengths := make([]int, 0, len(result.Categories))
		for _, category := range result.Categories {
			lengths = append(lengths, len(category))
		}
		t.Fatalf("categories lengths=%v want=[80 81 200]", lengths)
	}
	if !reflect.DeepEqual(result.Tags, []string{expectedTag}) || utf8.RuneCountInString(result.Tags[0]) != 64 {
		t.Fatalf("tags=%q runes=%d", result.Tags, utf8.RuneCountInString(result.Tags[0]))
	}
	if store.writeCalls != 0 {
		t.Fatalf("write calls=%d", store.writeCalls)
	}
}

func TestImageFacetsPreservesPerRowLongTagLimitQuirk(t *testing.T) {
	prefix := strings.Repeat("界", 64)
	parts := make([]string, 0, 51)
	for index := 0; index < 25; index++ {
		parts = append(parts, prefix+"甲")
	}
	for index := 0; index < 25; index++ {
		parts = append(parts, prefix+"乙")
	}
	parts = append(parts, "blocked-by-row-limit")
	store := &facetTestStore{rows: []FacetRow{
		{Tags: strings.Join(parts, ",")},
		{Tags: prefix + "丙,available-from-next-row"},
	}}
	service := NewService(&facetTestUOW{}, store, nil)

	result, err := service.Facets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"available-from-next-row", prefix}; !reflect.DeepEqual(result.Tags, want) {
		t.Fatalf("tags=%q want=%q", result.Tags, want)
	}
	if store.writeCalls != 0 {
		t.Fatalf("write calls=%d", store.writeCalls)
	}
}

func TestImageFacetsReturnsNonNullEmptySlicesAndCanonicalApplicationError(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		store := &facetTestStore{}
		result, err := NewService(&facetTestUOW{}, store, nil).Facets(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if result.Categories == nil || result.Tags == nil || len(result.Categories) != 0 || len(result.Tags) != 0 {
			t.Fatalf("result=%#v", result)
		}
		if store.readCalls != 1 || store.writeCalls != 0 {
			t.Fatalf("reads=%d writes=%d", store.readCalls, store.writeCalls)
		}
	})

	t.Run("repository failure", func(t *testing.T) {
		raw := errors.New("pq: internal-marker-0358")
		store := &facetTestStore{err: raw}
		result, err := NewService(&facetTestUOW{}, store, nil).Facets(context.Background())
		if !errors.Is(err, ErrFacetsUnavailable) || errors.Is(err, raw) {
			t.Fatalf("err=%v", err)
		}
		if result.Categories == nil || result.Tags == nil || store.writeCalls != 0 {
			t.Fatalf("result=%#v writes=%d", result, store.writeCalls)
		}
	})
}
