package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	wecomcallback "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/callback"
)

const (
	externalContactEvent = "change_external_contact"
	addExternalContact   = "add_external_contact"
	addHalfExternal      = "add_half_external_contact"
)

// ExternalContactCallbackFact is the typed, privacy-safe portion of a
// verified WeCom change_external_contact callback. WelcomeCode, Source, and
// FailReason are deliberately represented only by their SHA-256 digests.
//
// It is intentionally an input fact: associating it with an acquisition asset
// or a customer belongs to the later Contact/Identity transaction.
type ExternalContactCallbackFact struct {
	CorpID             string
	ChangeType         string
	ExternalUserID     string
	UserID             string
	State              string
	OccurredAt         time.Time
	WelcomeCodePresent bool
	WelcomeCodeDigest  string
	SourceDigest       string
	FailReasonDigest   string
}

// IsEntrant reports whether this fact can begin the inbound-acquisition flow.
// All other lifecycle events remain auditable but must not be treated as a new
// entrant.
func (fact ExternalContactCallbackFact) IsEntrant() bool {
	return (fact.ChangeType == addExternalContact || fact.ChangeType == addHalfExternal) && fact.UserID != ""
}

// EventType is stable for durable-inbox routing without exposing any callback
// payload value.
func (fact ExternalContactCallbackFact) EventType() string {
	return externalContactEvent + ":" + fact.ChangeType
}

// ParseExternalContactCallbackFact parses only an already authenticated and
// decrypted callback body. It does not perform WeCom signature validation,
// decryption, persistence, or customer attribution.
func ParseExternalContactCallbackFact(message []byte, corpID string) (ExternalContactCallbackFact, error) {
	if len(message) == 0 || len(message) > 1<<20 || !utf8.Valid(message) || !validCorpID(corpID) {
		return ExternalContactCallbackFact{}, ErrInvalidInboundMessage
	}

	var payload struct {
		XMLName         xml.Name `xml:"xml"`
		ToUserName      string   `xml:"ToUserName"`
		CreateTime      string   `xml:"CreateTime"`
		MsgType         string   `xml:"MsgType"`
		Event           string   `xml:"Event"`
		ChangeType      string   `xml:"ChangeType"`
		ExternalUserID  string   `xml:"ExternalUserID"`
		ExternalUserID2 string   `xml:"ExternalUserId"`
		UserID          string   `xml:"UserID"`
		State           string   `xml:"State"`
		WelcomeCode     string   `xml:"WelcomeCode"`
		Source          string   `xml:"Source"`
		FailReason      string   `xml:"FailReason"`
	}
	decoder := xml.NewDecoder(strings.NewReader(string(message)))
	if err := decoder.Decode(&payload); err != nil {
		return ExternalContactCallbackFact{}, invalidExternalContactCallback(err)
	}
	if err := rejectTrailingXML(decoder); err != nil {
		return ExternalContactCallbackFact{}, invalidExternalContactCallback(err)
	}
	if payload.XMLName.Local != "xml" || payload.ToUserName != corpID || payload.MsgType != "event" || payload.Event != externalContactEvent {
		return ExternalContactCallbackFact{}, invalidExternalContactCallback(nil)
	}
	occurredAt, err := parseCallbackOccurredAt(payload.CreateTime)
	if err != nil {
		return ExternalContactCallbackFact{}, invalidExternalContactCallback(err)
	}
	externalUserID, err := singleExternalUserID(payload.ExternalUserID, payload.ExternalUserID2)
	if err != nil || !validText(externalUserID, 1024) || !validCallbackLabel(payload.ChangeType) {
		return ExternalContactCallbackFact{}, invalidExternalContactCallback(err)
	}

	if !validOptionalCallbackValue(payload.State) {
		return ExternalContactCallbackFact{}, invalidExternalContactCallback(nil)
	}
	fact := ExternalContactCallbackFact{
		CorpID:         corpID,
		ChangeType:     payload.ChangeType,
		ExternalUserID: externalUserID,
		OccurredAt:     occurredAt,
		State:          payload.State,
	}
	if payload.UserID != "" {
		if !validText(payload.UserID, 1024) {
			return ExternalContactCallbackFact{}, invalidExternalContactCallback(nil)
		}
		fact.UserID = payload.UserID
	}
	if payload.WelcomeCode != "" {
		if !validSecretCallbackValue(payload.WelcomeCode) {
			return ExternalContactCallbackFact{}, invalidExternalContactCallback(nil)
		}
		fact.WelcomeCodePresent = true
		fact.WelcomeCodeDigest = callbackValueDigest(payload.WelcomeCode)
	}
	if payload.Source != "" {
		if !validSecretCallbackValue(payload.Source) {
			return ExternalContactCallbackFact{}, invalidExternalContactCallback(nil)
		}
		fact.SourceDigest = callbackValueDigest(payload.Source)
	}
	if payload.FailReason != "" {
		if !validSecretCallbackValue(payload.FailReason) {
			return ExternalContactCallbackFact{}, invalidExternalContactCallback(nil)
		}
		fact.FailReasonDigest = callbackValueDigest(payload.FailReason)
	}
	return fact, nil
}

func rejectTrailingXML(decoder *xml.Decoder) error {
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if text, ok := token.(xml.CharData); ok && strings.TrimSpace(string(text)) == "" {
			continue
		}
		return errors.New("trailing XML content")
	}
}

func invalidExternalContactCallback(cause error) error {
	return errors.Join(ErrInvalidInboundMessage, wecomcallback.ErrUnknownCallbackEvent, cause)
}

func parseCallbackOccurredAt(value string) (time.Time, error) {
	if !validText(value, 32) {
		return time.Time{}, errors.New("invalid create time")
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds <= 0 {
		return time.Time{}, errors.New("invalid create time")
	}
	return time.Unix(seconds, 0).UTC(), nil
}

func singleExternalUserID(primary, compatibility string) (string, error) {
	if primary != "" && compatibility != "" && primary != compatibility {
		return "", errors.New("conflicting external user IDs")
	}
	if primary != "" {
		return primary, nil
	}
	return compatibility, nil
}

func validCallbackLabel(value string) bool {
	if !validText(value, 128) {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '_') {
			return false
		}
	}
	return true
}

func validOptionalCallbackValue(value string) bool {
	return value == "" || validText(value, 1024)
}

func validSecretCallbackValue(value string) bool {
	return len(value) <= 4096 && utf8.ValidString(value) && strings.TrimSpace(value) == value
}

func callbackValueDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(digest[:])
}
