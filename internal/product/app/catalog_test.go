package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type productTestUoW struct{ calls int }

func (u *productTestUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	u.calls++
	return callback(context.WithValue(ctx, productTestTxKey{}, true))
}

type productTestTxKey struct{}

type productTestStore struct {
	products      []productport.Product
	receipt       Receipt
	reserved      bool
	createCalls   int
	listOffset    int32
	listLimit     int32
	countCalls    int
	completeCalls int
}

func (s *productTestStore) List(_ context.Context, after *productport.ID, limit int32) ([]productport.Product, error) {
	items := append([]productport.Product(nil), s.products...)
	if after != nil {
		filtered := items[:0]
		for _, item := range items {
			if item.ID > *after {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if len(items) > int(limit) {
		items = items[:limit]
	}
	return items, nil
}
func (s *productTestStore) ListOffset(_ context.Context, limit, offset int32) ([]productport.Product, error) {
	s.listLimit, s.listOffset = limit, offset
	if int(offset) >= len(s.products) {
		return []productport.Product{}, nil
	}
	end := int(offset + limit)
	if end > len(s.products) {
		end = len(s.products)
	}
	return append([]productport.Product(nil), s.products[offset:end]...), nil
}
func (s *productTestStore) Count(context.Context) (int64, error) {
	s.countCalls++
	return int64(len(s.products)), nil
}
func (s *productTestStore) Get(_ context.Context, id productport.ID) (productport.Product, error) {
	for _, product := range s.products {
		if product.ID == id {
			return product, nil
		}
	}
	return productport.Product{}, ErrNotFound
}
func (s *productTestStore) Create(_ context.Context, command productport.CreateCommand, now time.Time) (productport.Product, error) {
	s.createCalls++
	product := productport.Product{ID: productport.ID(len(s.products) + 1), ProductCode: command.ProductCode, Name: command.Name,
		Description: command.Description, PriceMinor: command.PriceMinor, Currency: command.Currency,
		StockQuantity: command.StockQuantity, Images: append([]string(nil), command.Images...), CreatedBy: command.Actor,
		CreatedAt: now, UpdatedAt: now, LegacyAdminProjection: append([]byte(nil), command.LegacyAdminProjection...)}
	s.products = append(s.products, product)
	return product, nil
}
func (s *productTestStore) Reserve(_ context.Context, reservation Reservation) (Receipt, bool, error) {
	if !s.reserved {
		s.reserved = true
		s.receipt = Receipt{ID: 1, ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest,
			PayloadDigest: reservation.PayloadDigest, State: "in_progress"}
		return s.receipt, true, nil
	}
	return s.receipt, false, nil
}
func (s *productTestStore) Complete(_ context.Context, _ int64, snapshot json.RawMessage, _ time.Time) (Receipt, error) {
	s.completeCalls++
	var value any
	if json.Unmarshal(snapshot, &value) != nil {
		return Receipt{}, ErrUnavailable
	}
	canonical, _ := json.Marshal(value)
	s.receipt.State = "completed"
	s.receipt.ResultSnapshot = canonical
	return s.receipt, nil
}

type productTestEvents struct {
	events []eventport.Event
	err    error
}

func (e *productTestEvents) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	if e.err != nil {
		return 0, e.err
	}
	e.events = append(e.events, event)
	return eventport.EventID(len(e.events)), nil
}

func validTestProduct(id int64) productport.Product {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	return productport.Product{ID: productport.ID(id), ProductCode: "sku-" + string(rune('a'+id)), Name: "商品",
		Description: "说明", PriceMinor: 9900, Currency: "CNY", StockQuantity: 0, Images: []string{},
		CreatedBy: 7, CreatedAt: now, UpdatedAt: now, LegacyAdminProjection: DefaultLegacyAdminProjection()}
}

func TestCreateCanonicalLegacyProjectionParticipatesInIdempotency(t *testing.T) {
	uow, store, events := &productTestUoW{}, &productTestStore{}, &productTestEvents{}
	service := NewService(uow, store, events)
	service.now = func() time.Time { return time.Date(2026, 8, 14, 12, 1, 0, 0, time.UTC) }
	command := productport.CreateCommand{ProductCode: "sku-001", Name: "普通商品", Description: "说明", PriceMinor: 1999,
		Currency: "cny", StockQuantity: 0, Images: []string{"https://img.example/a.png"}, Actor: 7,
		IdempotencyKey: "product-idempotency-001", LegacyAdminProjection: json.RawMessage(`{"enabled":true,"status":"active","schema_version":1,"slices":[{"image_id":8}]}`)}

	first, err := service.Create(context.Background(), command)
	if err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	command.LegacyAdminProjection = json.RawMessage(`{"schema_version":1,"slices":[{"image_id":8}],"status":"active","enabled":true}`)
	replayed, err := service.Create(context.Background(), command)
	if err != nil || replayed.ID != first.ID || store.createCalls != 1 || len(events.events) != 1 || store.completeCalls != 1 {
		t.Fatalf("replay product=%+v err=%v creates=%d events=%d completes=%d", replayed, err, store.createCalls, len(events.events), store.completeCalls)
	}
	command.LegacyAdminProjection = json.RawMessage(`{"schema_version":1,"slices":[{"image_id":9}],"status":"active","enabled":true}`)
	if _, err = service.Create(context.Background(), command); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed projection error = %v, want conflict", err)
	}
	if events.events[0].Type != eventport.EvProductCreated || string(events.events[0].Payload) != `{"product_id":1,"actor":7}` {
		t.Fatalf("event = %+v", events.events[0])
	}
}

func TestListLegacyReturnsBoundedOffsetPageAndExactTotal(t *testing.T) {
	store := &productTestStore{products: []productport.Product{validTestProduct(1), validTestProduct(2), validTestProduct(3)}}
	service := NewService(&productTestUoW{}, store, &productTestEvents{})
	page, err := service.ListLegacy(context.Background(), 1, 1)
	if err != nil || page.Total != 3 || page.Limit != 1 || page.Offset != 1 || len(page.Items) != 1 || page.Items[0].ID != 2 {
		t.Fatalf("ListLegacy() = %+v, %v", page, err)
	}
	if store.listLimit != 1 || store.listOffset != 1 || store.countCalls != 1 {
		t.Fatalf("store list/count = %d/%d/%d", store.listLimit, store.listOffset, store.countCalls)
	}
	for _, input := range [][2]int32{{0, 0}, {101, 0}, {1, -1}, {1, MaximumLegacyOffset + 1}} {
		if _, err = service.ListLegacy(context.Background(), input[0], input[1]); !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("ListLegacy(%d,%d) error = %v", input[0], input[1], err)
		}
	}
}

func TestCanonicalLegacyAdminProjectionRejectsUnknownAndWrongTypes(t *testing.T) {
	for _, raw := range []string{
		`{"schema_version":2}`,
		`{"schema_version":1,"unknown_field":"not-supported"}`,
		`{"schema_version":1,"enabled":"yes"}`,
		`{"schema_version":1,"slices":{}}`,
		`{"schema_version":1,"completion_target":3}`,
		`{"schema_version":1,"wecom_tagging":true}`,
		`{"schema_version":1,"lead_program_id":0}`,
		`{"schema_version":1} 42`,
	} {
		if _, err := CanonicalLegacyAdminProjection(json.RawMessage(raw)); !errors.Is(err, ErrInvalidProduct) {
			t.Fatalf("projection %s error = %v", raw, err)
		}
	}
	projection := DefaultLegacyAdminProjection()
	if string(projection) == "" || !json.Valid(projection) {
		t.Fatalf("default projection = %s", projection)
	}
	explicitNulls, err := CanonicalLegacyAdminProjection(json.RawMessage(`{"schema_version":1,"lead_program_id":null,"lead_channel_id":null,"completion_target":null}`))
	if err != nil || !jsonEquivalent(projection, explicitNulls) {
		t.Fatalf("missing fields must equal their frozen null defaults: projection=%s explicit=%s error=%v", projection, explicitNulls, err)
	}
	objectTarget, err := CanonicalLegacyAdminProjection(json.RawMessage(`{"schema_version":1,"completion_target":{}}`))
	if err != nil || jsonEquivalent(explicitNulls, objectTarget) {
		t.Fatalf("null and object targets must remain distinct: nulls=%s object=%s error=%v", explicitNulls, objectTarget, err)
	}
	if jsonEquivalent([]byte(`{"lead_program_id":9007199254740992}`), []byte(`{"lead_program_id":9007199254740993}`)) {
		t.Fatal("JSON semantic comparison lost integer precision above 2^53")
	}
}
