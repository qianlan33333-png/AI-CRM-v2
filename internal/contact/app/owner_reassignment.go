package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const OwnerReassignmentConfirmation = "CONFIRM OWNER REASSIGNMENT"
const ownerReassignmentPreviewTTL = 15 * time.Minute
const ownerReassignmentMaxBytes = 1 << 20
const ownerReassignmentMaxRows = 500

var (
	ErrOwnerReassignmentInvalid     = errors.New("owner reassignment invalid")
	ErrOwnerReassignmentNotFound    = errors.New("owner reassignment preview not found")
	ErrOwnerReassignmentConflict    = errors.New("owner reassignment conflict")
	ErrOwnerReassignmentExpired     = errors.New("owner reassignment preview expired")
	ErrOwnerReassignmentUnavailable = errors.New("owner reassignment unavailable")
)

type OwnerReassignmentRow struct {
	CustomerID           int64     `json:"customer_id"`
	ExpectedOwnerStaffID int64     `json:"expected_owner_staff_id"`
	ExpectedUpdatedAt    time.Time `json:"expected_updated_at"`
	TargetOwnerStaffID   int64     `json:"target_owner_staff_id"`
}
type OwnerReassignmentResultRow struct {
	CustomerID           int64     `json:"customer_id"`
	PreviousOwnerStaffID int64     `json:"previous_owner_staff_id"`
	TargetOwnerStaffID   int64     `json:"target_owner_staff_id"`
	UpdatedAt            time.Time `json:"updated_at"`
}
type OwnerReassignmentIssue struct {
	Line int    `json:"line"`
	Code string `json:"code"`
}
type OwnerReassignmentPreview struct {
	ID        string                       `json:"id"`
	Hash      string                       `json:"hash"`
	Rows      []OwnerReassignmentRow       `json:"rows"`
	Issues    []OwnerReassignmentIssue     `json:"issues"`
	ExpiresAt time.Time                    `json:"expires_at"`
	Executed  bool                         `json:"executed"`
	Result    []OwnerReassignmentResultRow `json:"result,omitempty"`
}
type OwnerReassignmentReceipt struct {
	ID            int64
	PayloadDigest []byte
	Completed     bool
	Result        []OwnerReassignmentResultRow
}

type OwnerReassignmentStore interface {
	CreateOwnerReassignmentPreview(context.Context, OwnerReassignmentPreview, int64, []byte, []byte, time.Time) (OwnerReassignmentPreview, bool, error)
	ReadOwnerReassignmentPreview(context.Context, string, int64) (OwnerReassignmentPreview, error)
	ReserveOwnerReassignmentReceipt(context.Context, int64, []byte, []byte, time.Time) (OwnerReassignmentReceipt, bool, error)
	LockOwnerReassignmentPreview(context.Context, string, int64, []byte, time.Time) (OwnerReassignmentPreview, error)
	LockActiveOwnerReassignmentStaff(context.Context, int64) error
	LockOwnerReassignmentCustomer(context.Context, int64) (OwnerReassignmentRow, error)
	UpdateOwnerReassignmentCustomer(context.Context, int64, int64) (time.Time, error)
	AppendOwnerReassignmentCustomerEvent(context.Context, int64, []byte, int64, time.Time) error
	MarkOwnerReassignmentPreviewExecuted(context.Context, string, []OwnerReassignmentResultRow, time.Time) error
	CompleteOwnerReassignmentReceipt(context.Context, int64, []OwnerReassignmentResultRow, time.Time) error
}

type OwnerReassignmentService struct {
	uow    platformport.UnitOfWork
	store  OwnerReassignmentStore
	events eventport.Appender
	now    func() time.Time
}

func NewOwnerReassignmentService(uow platformport.UnitOfWork, store OwnerReassignmentStore, events eventport.Appender) *OwnerReassignmentService {
	return &OwnerReassignmentService{uow: uow, store: store, events: events, now: time.Now}
}

func OwnerReassignmentTemplate() []byte {
	return []byte("customer_id,expected_owner_staff_id,expected_updated_at,target_owner_staff_id\r\n")
}

func (s *OwnerReassignmentService) CreatePreview(ctx context.Context, actor int64, data []byte, key string) (OwnerReassignmentPreview, error) {
	if !ownerReassignmentReady(s) || actor < 1 || !validOwnerReassignmentKey(key) {
		return OwnerReassignmentPreview{}, ErrOwnerReassignmentInvalid
	}
	rows, issues, err := parseOwnerReassignmentCSV(data)
	if err != nil {
		return OwnerReassignmentPreview{}, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	id, err := ownerReassignmentID()
	if err != nil {
		return OwnerReassignmentPreview{}, ErrOwnerReassignmentUnavailable
	}
	digest := sha256.Sum256(data)
	actorBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(actorBytes, uint64(actor))
	hash := sha256.Sum256(append(append([]byte(id+":"), digest[:]...), actorBytes...))
	p := OwnerReassignmentPreview{ID: id, Hash: hex.EncodeToString(hash[:]), Rows: rows, Issues: issues, ExpiresAt: now.Add(ownerReassignmentPreviewTTL)}
	keyDigest := sha256.Sum256([]byte(key))
	if err = s.uow.Within(ctx, func(tx context.Context) error {
		stored, owned, createErr := s.store.CreateOwnerReassignmentPreview(tx, p, actor, digest[:], keyDigest[:], now)
		if createErr != nil {
			return createErr
		}
		if !owned {
			p = stored
		}
		return nil
	}); err != nil {
		return OwnerReassignmentPreview{}, mapOwnerReassignmentError(err)
	}
	return p, nil
}

func (s *OwnerReassignmentService) Preview(ctx context.Context, actor int64, id string) (OwnerReassignmentPreview, error) {
	if !ownerReassignmentReady(s) || actor < 1 || !validOwnerReassignmentID(id) {
		return OwnerReassignmentPreview{}, ErrOwnerReassignmentInvalid
	}
	var p OwnerReassignmentPreview
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var e error
		p, e = s.store.ReadOwnerReassignmentPreview(tx, id, actor)
		return e
	})
	if err == nil && !p.Executed && s.now().UTC().After(p.ExpiresAt) {
		return OwnerReassignmentPreview{}, ErrOwnerReassignmentExpired
	}
	return p, mapOwnerReassignmentError(err)
}

func (s *OwnerReassignmentService) Execute(ctx context.Context, actor int64, previewID, previewHash, confirmation, key string) (OwnerReassignmentPreview, error) {
	if !ownerReassignmentReady(s) || actor < 1 || !validOwnerReassignmentID(previewID) || len(previewHash) != 64 || confirmation != OwnerReassignmentConfirmation || !validOwnerReassignmentKey(key) {
		return OwnerReassignmentPreview{}, ErrOwnerReassignmentInvalid
	}
	payload, _ := json.Marshal(struct{ PreviewID, PreviewHash, Confirmation string }{previewID, previewHash, confirmation})
	pd := sha256.Sum256(payload)
	kd := sha256.Sum256([]byte(key))
	now := s.now().UTC().Truncate(time.Microsecond)
	var result OwnerReassignmentPreview
	err := s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, e := s.store.ReserveOwnerReassignmentReceipt(tx, actor, kd[:], pd[:], now)
		if e != nil {
			return e
		}
		if !bytes.Equal(receipt.PayloadDigest, pd[:]) {
			return ErrOwnerReassignmentConflict
		}
		if !owned {
			if !receipt.Completed {
				return ErrOwnerReassignmentConflict
			}
			p, readErr := s.store.ReadOwnerReassignmentPreview(tx, previewID, actor)
			if readErr != nil {
				return readErr
			}
			p.Result = receipt.Result
			p.Executed = true
			result = p
			return nil
		}
		p, e := s.store.LockOwnerReassignmentPreview(tx, previewID, actor, decodeOwnerReassignmentHash(previewHash), now)
		if e != nil {
			return e
		}
		if len(p.Issues) > 0 {
			return ErrOwnerReassignmentConflict
		}
		ids := append([]OwnerReassignmentRow(nil), p.Rows...)
		sort.Slice(ids, func(i, j int) bool { return ids[i].CustomerID < ids[j].CustomerID })
		targets := make([]int64, 0, len(ids))
		for _, row := range ids {
			if len(targets) == 0 || targets[len(targets)-1] != row.TargetOwnerStaffID {
				targets = append(targets, row.TargetOwnerStaffID)
			}
		}
		sort.Slice(targets, func(i, j int) bool { return targets[i] < targets[j] })
		dedup := targets[:0]
		for _, target := range targets {
			if len(dedup) == 0 || dedup[len(dedup)-1] != target {
				dedup = append(dedup, target)
			}
		}
		for _, target := range dedup {
			if e = s.store.LockActiveOwnerReassignmentStaff(tx, target); e != nil {
				return e
			}
		}
		for _, row := range ids {
			current, lockErr := s.store.LockOwnerReassignmentCustomer(tx, row.CustomerID)
			if lockErr != nil {
				return lockErr
			}
			if current.ExpectedOwnerStaffID != row.ExpectedOwnerStaffID || !current.ExpectedUpdatedAt.Equal(row.ExpectedUpdatedAt) {
				return ErrOwnerReassignmentConflict
			}
		}
		updated := make([]OwnerReassignmentResultRow, 0, len(ids))
		for _, row := range ids {
			at, updateErr := s.store.UpdateOwnerReassignmentCustomer(tx, row.CustomerID, row.TargetOwnerStaffID)
			if updateErr != nil {
				return updateErr
			}
			updated = append(updated, OwnerReassignmentResultRow{CustomerID: row.CustomerID, PreviousOwnerStaffID: row.ExpectedOwnerStaffID, TargetOwnerStaffID: row.TargetOwnerStaffID, UpdatedAt: at})
			payload, marshalErr := json.Marshal(struct {
				CustomerID int64 `json:"customer_id"`
				Actor      int64 `json:"actor"`
			}{row.CustomerID, actor})
			if marshalErr != nil {
				return marshalErr
			}
			if appendErr := s.store.AppendOwnerReassignmentCustomerEvent(tx, row.CustomerID, payload, actor, now); appendErr != nil {
				return appendErr
			}
			if _, appendErr := s.events.Append(tx, eventport.Event{Type: eventport.EvCustomerUpdated, CustomerID: eventport.CustomerID(row.CustomerID), Payload: payload, OccurredAt: now, IdempotencyKey: "contact.owner_reassignment:" + previewID + ":" + strconv.FormatInt(row.CustomerID, 10)}); appendErr != nil {
				return appendErr
			}
		}
		if e = s.store.MarkOwnerReassignmentPreviewExecuted(tx, previewID, updated, now); e != nil {
			return e
		}
		if e = s.store.CompleteOwnerReassignmentReceipt(tx, receipt.ID, updated, now); e != nil {
			return e
		}
		result = p
		result.Result = updated
		result.Executed = true
		return nil
	})
	if err != nil {
		return OwnerReassignmentPreview{}, mapOwnerReassignmentError(err)
	}
	return result, nil
}

func parseOwnerReassignmentCSV(data []byte) ([]OwnerReassignmentRow, []OwnerReassignmentIssue, error) {
	if len(data) == 0 || len(data) > ownerReassignmentMaxBytes || !utf8.Valid(data) {
		return nil, nil, ErrOwnerReassignmentInvalid
	}
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})))
	r.FieldsPerRecord = 4
	r.ReuseRecord = false
	header, e := r.Read()
	if e != nil || strings.Join(header, ",") != "customer_id,expected_owner_staff_id,expected_updated_at,target_owner_staff_id" {
		return nil, nil, ErrOwnerReassignmentInvalid
	}
	rows := []OwnerReassignmentRow{}
	issues := []OwnerReassignmentIssue{}
	seen := map[int64]bool{}
	for line := 2; ; line++ {
		record, er := r.Read()
		if er == io.EOF {
			break
		}
		if er != nil {
			return nil, nil, ErrOwnerReassignmentInvalid
		}
		if len(rows)+len(issues) >= ownerReassignmentMaxRows {
			return nil, nil, ErrOwnerReassignmentInvalid
		}
		var row OwnerReassignmentRow
		var ok = true
		vals := []*int64{&row.CustomerID, &row.ExpectedOwnerStaffID, nil, &row.TargetOwnerStaffID}
		for i, target := range vals {
			if strings.HasPrefix(record[i], "=") || strings.HasPrefix(record[i], "+") || strings.HasPrefix(record[i], "-") || strings.HasPrefix(record[i], "@") {
				ok = false
				break
			}
			if target != nil {
				v, x := strconv.ParseInt(record[i], 10, 64)
				if x != nil || v < 1 {
					ok = false
					break
				}
				*target = v
			}
		}
		if parsed, x := time.Parse(time.RFC3339Nano, record[2]); x != nil || parsed.IsZero() {
			ok = false
		} else {
			row.ExpectedUpdatedAt = parsed.UTC().Truncate(time.Microsecond)
		}
		if seen[row.CustomerID] || row.TargetOwnerStaffID == row.ExpectedOwnerStaffID {
			ok = false
		}
		seen[row.CustomerID] = true
		if !ok {
			issues = append(issues, OwnerReassignmentIssue{Line: line, Code: "invalid_row"})
		} else {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return nil, issues, ErrOwnerReassignmentInvalid
	}
	return rows, issues, nil
}
func ownerReassignmentReady(s *OwnerReassignmentService) bool {
	return s != nil && s.uow != nil && s.store != nil && s.events != nil && s.now != nil
}
func ownerReassignmentID() (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return "cor_" + base64.RawURLEncoding.EncodeToString(b), nil
}
func validOwnerReassignmentID(v string) bool { return len(v) == 26 && strings.HasPrefix(v, "cor_") }
func validOwnerReassignmentKey(v string) bool {
	return len(v) >= 8 && len(v) <= 200 && strings.TrimSpace(v) == v
}
func decodeOwnerReassignmentHash(v string) []byte { b, _ := hex.DecodeString(v); return b }
func mapOwnerReassignmentError(e error) error {
	if e == nil {
		return nil
	}
	if errors.Is(e, ErrOwnerReassignmentInvalid) || errors.Is(e, ErrOwnerReassignmentNotFound) || errors.Is(e, ErrOwnerReassignmentConflict) || errors.Is(e, ErrOwnerReassignmentExpired) {
		return e
	}
	return errors.Join(ErrOwnerReassignmentUnavailable, e)
}
