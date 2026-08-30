// Package archive owns read-only WeCom Finance SDK ingestion and the local
// message-archive projection. It never sends a message or mutates WeCom.
package archive

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	JobKind       = "wecom_message_archive_sync_v1"
	DefaultLimit  = 100
	DefaultPages  = 200
	SyncPeriod    = 3 * time.Minute
	maximumText   = 20_000
	maximumPeerID = 1_024
)

var (
	ErrInvalidConfiguration = errors.New("invalid WeCom message archive configuration")
	ErrSyncUnavailable      = errors.New("WeCom message archive sync unavailable")
)

type EncryptedRecord struct {
	Seq              int64  `json:"seq"`
	PublicKeyVersion int    `json:"publickey_ver"`
	EncryptedKey     string `json:"encrypt_random_key"`
	EncryptedMessage string `json:"encrypt_chat_msg"`
}

type Provider interface {
	FetchPage(context.Context, int64, int) ([]EncryptedRecord, error)
	Decrypt(context.Context, []EncryptedRecord) ([]map[string]any, error)
}

type IdentityResolver interface {
	Resolve(context.Context, identityport.IDRef) (identityport.ResolveResult, error)
}

type State struct {
	LastSeq int64
}

type Record struct {
	SourceMessageID, ExternalUserID, ChatType, OwnerUserID   string
	Sender, Receiver, ChatID, RoomID, GroupName, MessageType string
	Content                                                  string
	CustomerID                                               *contactport.CustomerID
	ProviderSeq                                              int64
	SentAt                                                   time.Time
	SourcePayloadDigest                                      [32]byte
}

type RunResult struct {
	RunID, CursorFrom, CursorTo                        int64
	Fetched, Accepted, Inserted, Unresolved, PageCount int64
}

type Store interface {
	State(context.Context) (State, error)
	StartRun(context.Context, int64) (int64, error)
	SaveBatch(context.Context, []Record, int64, time.Time) (inserted, unresolved int64, err error)
	FinishRun(context.Context, RunResult, string, time.Time) error
	ResolvePending(context.Context, string) (int64, error)
}

type Service struct {
	uow          platformport.UnitOfWork
	store        Store
	provider     Provider
	identities   IdentityResolver
	events       eventport.Appender
	corpID       string
	defaultOwner string
	limit        int
	maxPages     int
	now          func() time.Time
}

func NewService(uow platformport.UnitOfWork, store Store, provider Provider, identities IdentityResolver, events eventport.Appender, corpID, defaultOwner string, limit, maxPages int) (*Service, error) {
	if uow == nil || store == nil || provider == nil || identities == nil || events == nil || !validID(corpID) || defaultOwner != "" && !validID(defaultOwner) || limit < 1 || limit > 1_000 || maxPages < 1 || maxPages > 10_000 {
		return nil, ErrInvalidConfiguration
	}
	return &Service{uow: uow, store: store, provider: provider, identities: identities, events: events, corpID: corpID, defaultOwner: defaultOwner, limit: limit, maxPages: maxPages, now: time.Now}, nil
}

func (service *Service) Sync(ctx context.Context) (result RunResult, err error) {
	if service == nil || ctx == nil {
		return RunResult{}, ErrSyncUnavailable
	}
	state, err := service.readState(ctx)
	if err != nil || state.LastSeq < 0 {
		return RunResult{}, errors.Join(ErrSyncUnavailable, err)
	}
	result.CursorFrom, result.CursorTo = state.LastSeq, state.LastSeq
	result.RunID, err = service.startRun(ctx, state.LastSeq)
	if err != nil {
		return RunResult{}, errors.Join(ErrSyncUnavailable, err)
	}
	defer func() {
		failureCode := ""
		if err != nil {
			failureCode = failureCodeFor(err)
		}
		if finishErr := service.finishRun(context.WithoutCancel(ctx), result, failureCode); finishErr != nil {
			err = errors.Join(err, finishErr)
		}
	}()

	for page := 0; page < service.maxPages; page++ {
		encrypted, fetchErr := service.provider.FetchPage(ctx, result.CursorTo, service.limit)
		if fetchErr != nil {
			return result, errors.Join(ErrSyncUnavailable, fetchErr)
		}
		if len(encrypted) == 0 {
			break
		}
		decrypted, decryptErr := service.provider.Decrypt(ctx, encrypted)
		if decryptErr != nil || len(decrypted) != len(encrypted) {
			return result, errors.Join(ErrSyncUnavailable, decryptErr)
		}
		result.Fetched += int64(len(encrypted))
		result.PageCount++
		records := make([]Record, 0, len(encrypted))
		for index, envelope := range encrypted {
			if envelope.Seq <= result.CursorTo {
				return result, ErrSyncUnavailable
			}
			result.CursorTo = envelope.Seq
			record, accepted := normalizeRecord(envelope, decrypted[index], service.defaultOwner)
			if !accepted {
				continue
			}
			result.Accepted++
			if record.ExternalUserID != "" {
				resolved, resolveErr := service.identities.Resolve(ctx, identityport.IDRef{
					Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:" + service.corpID,
					Value: record.ExternalUserID, Assurance: identityport.AssuranceVerified, Source: "wecom.message_archive",
				})
				if resolveErr != nil {
					return result, errors.Join(ErrSyncUnavailable, resolveErr)
				}
				if resolved.Status == identityport.ResolveFound && resolved.CustomerID > 0 {
					customerID := resolved.CustomerID
					record.CustomerID = &customerID
				}
			}
			records = append(records, record)
		}
		inserted, unresolved, saveErr := service.saveBatch(ctx, records, result.CursorTo)
		if saveErr != nil {
			return result, errors.Join(ErrSyncUnavailable, saveErr)
		}
		result.Inserted += inserted
		result.Unresolved += unresolved
		if len(encrypted) < service.limit {
			break
		}
	}
	_, err = service.resolvePending(ctx)
	if err != nil {
		return result, errors.Join(ErrSyncUnavailable, err)
	}
	return result, nil
}

func (service *Service) readState(ctx context.Context) (state State, err error) {
	err = service.uow.Within(ctx, func(tx context.Context) error {
		state, err = service.store.State(tx)
		return err
	})
	return state, err
}

func (service *Service) startRun(ctx context.Context, cursor int64) (runID int64, err error) {
	err = service.uow.Within(ctx, func(tx context.Context) error {
		runID, err = service.store.StartRun(tx, cursor)
		return err
	})
	return runID, err
}

func (service *Service) saveBatch(ctx context.Context, records []Record, cursor int64) (inserted, unresolved int64, err error) {
	now := service.now().UTC()
	err = service.uow.Within(ctx, func(tx context.Context) error {
		inserted, unresolved, err = service.store.SaveBatch(tx, records, cursor, now)
		if err != nil {
			return err
		}
		payload, marshalErr := json.Marshal(map[string]any{"cursor": cursor, "accepted": len(records), "inserted": inserted, "unresolved": unresolved})
		if marshalErr != nil {
			return marshalErr
		}
		_, err = service.events.Append(tx, eventport.Event{Type: "wecom.message_archive_batch_persisted", Payload: payload, OccurredAt: now, IdempotencyKey: fmt.Sprintf("wecom.message_archive.batch:%d", cursor)})
		return err
	})
	return inserted, unresolved, err
}

func (service *Service) finishRun(ctx context.Context, result RunResult, failureCode string) error {
	return service.uow.Within(ctx, func(tx context.Context) error {
		return service.store.FinishRun(tx, result, failureCode, service.now().UTC())
	})
}

func (service *Service) resolvePending(ctx context.Context) (resolved int64, err error) {
	err = service.uow.Within(ctx, func(tx context.Context) error {
		resolved, err = service.store.ResolvePending(tx, "wecom-corp:"+service.corpID)
		return err
	})
	return resolved, err
}

func normalizeRecord(envelope EncryptedRecord, payload map[string]any, fallbackOwner string) (Record, bool) {
	if envelope.Seq < 1 || stringValue(payload["msgtype"]) != "text" {
		return Record{}, false
	}
	sender := stringValue(payload["from"])
	recipients := stringSlice(payload["tolist"])
	roomID := stringValue(payload["roomid"])
	externalID, ownerID := "", ""
	for _, candidate := range append([]string{sender}, recipients...) {
		if isExternalUserID(candidate) && externalID == "" {
			externalID = candidate
		} else if candidate != "" && !isExternalUserID(candidate) && ownerID == "" {
			ownerID = candidate
		}
	}
	if ownerID == "" {
		ownerID = fallbackOwner
	}
	text, _ := payload["text"].(map[string]any)
	content := strings.TrimSpace(stringValue(text["content"]))
	messageID := stringValue(payload["msgid"])
	if messageID == "" {
		messageID = "seq-" + strconv.FormatInt(envelope.Seq, 10)
	}
	sentAt, ok := messageTime(payload["msgtime"])
	if !ok || externalID == "" && roomID == "" || ownerID == "" && roomID == "" ||
		externalID != "" && !validID(externalID) || ownerID != "" && (!validID(ownerID) || len(ownerID) > 256) ||
		roomID != "" && !validID(roomID) || content == "" || len(content) > maximumText || len(strings.Join(recipients, ",")) > maximumPeerID {
		return Record{}, false
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Record{}, false
	}
	chatType := "private"
	if roomID != "" || len(recipients) != 1 {
		chatType = "group"
	}
	return Record{
		SourceMessageID: messageID, ExternalUserID: externalID, ChatType: chatType, OwnerUserID: ownerID,
		Sender: sender, Receiver: strings.Join(recipients, ","), ChatID: roomID, RoomID: roomID,
		MessageType: "text", Content: maskPhoneNumbers(content), ProviderSeq: envelope.Seq,
		SentAt: sentAt.UTC(), SourcePayloadDigest: sha256.Sum256(encoded),
	}, true
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) != "" {
			return []string{strings.TrimSpace(typed)}
		}
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := stringValue(item); value != "" {
				result = append(result, value)
			}
		}
		return result
	}
	return nil
}

func messageTime(value any) (time.Time, bool) {
	var milliseconds int64
	switch typed := value.(type) {
	case float64:
		milliseconds = int64(typed)
	case json.Number:
		milliseconds, _ = typed.Int64()
	case string:
		milliseconds, _ = strconv.ParseInt(typed, 10, 64)
	}
	if milliseconds < 1 {
		return time.Time{}, false
	}
	if milliseconds < 10_000_000_000 {
		milliseconds *= 1_000
	}
	return time.UnixMilli(milliseconds), true
}

func isExternalUserID(value string) bool {
	return strings.HasPrefix(value, "wm") || strings.HasPrefix(value, "wo")
}
func validID(value string) bool {
	return value != "" && len(value) <= maximumPeerID && strings.TrimSpace(value) == value
}

func maskPhoneNumbers(value string) string {
	var output strings.Builder
	for index := 0; index < len(value); {
		end := index
		if value[index] == '+' {
			end++
		}
		for end < len(value) && value[end] >= '0' && value[end] <= '9' {
			end++
		}
		digits := value[index:end]
		normalized := strings.TrimPrefix(digits, "+86")
		if len(normalized) == 11 && normalized[0] == '1' && normalized[1] >= '3' && normalized[1] <= '9' {
			output.WriteString("[masked-phone]")
			index = end
			continue
		}
		output.WriteByte(value[index])
		index++
	}
	return output.String()
}

func failureCodeFor(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	default:
		return "sync_failed"
	}
}
