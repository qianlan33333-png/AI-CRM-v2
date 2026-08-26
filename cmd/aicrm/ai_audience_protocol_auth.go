package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/legacyaudience"
)

type aiAudienceProtocolAuthenticator struct {
	key []byte
	now func() time.Time
}

func (authenticator *aiAudienceProtocolAuthenticator) Authenticate(ctx context.Context, request *http.Request, body []byte) (legacyaudience.InboundWebhookIdentity, error) {
	if authenticator == nil || ctx == nil || request == nil || authenticator.now == nil || len(authenticator.key) != sha256.Size {
		return legacyaudience.InboundWebhookIdentity{}, legacyaudience.ErrUnavailable
	}
	clientID, ok := singleAIAudienceHeader(request, "X-AICRM-Client-Id")
	if !ok || clientID != legacyaudience.AIAudienceWebhookClientID {
		return legacyaudience.InboundWebhookIdentity{}, legacyaudience.ErrUnauthenticated
	}
	timestamp, ok := singleAIAudienceHeader(request, "X-AICRM-Timestamp")
	if !ok {
		return legacyaudience.InboundWebhookIdentity{}, legacyaudience.ErrUnauthenticated
	}
	eventID, ok := singleAIAudienceHeader(request, "X-AICRM-Event-Id")
	if !ok || !validAIAudienceProtocolEventID(eventID) {
		return legacyaudience.InboundWebhookIdentity{}, legacyaudience.ErrUnauthenticated
	}
	signature, ok := singleAIAudienceHeader(request, "X-AICRM-Signature")
	if !ok {
		return legacyaudience.InboundWebhookIdentity{}, legacyaudience.ErrUnauthenticated
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || timestamp != strconv.FormatInt(seconds, 10) {
		return legacyaudience.InboundWebhookIdentity{}, legacyaudience.ErrUnauthenticated
	}
	now, signedAt := authenticator.now().UTC(), time.Unix(seconds, 0).UTC()
	if now.Sub(signedAt) > 5*time.Minute || signedAt.Sub(now) > time.Minute {
		return legacyaudience.InboundWebhookIdentity{}, legacyaudience.ErrUnauthenticated
	}
	rawSignature := strings.TrimPrefix(signature, "sha256=")
	decoded, err := hex.DecodeString(rawSignature)
	if err != nil || len(decoded) != sha256.Size || rawSignature != strings.ToLower(rawSignature) {
		return legacyaudience.InboundWebhookIdentity{}, legacyaudience.ErrUnauthenticated
	}
	mac := hmac.New(sha256.New, authenticator.key)
	_, _ = mac.Write([]byte(timestamp + "\n" + eventID + "\n"))
	_, _ = mac.Write(body)
	if !hmac.Equal(decoded, mac.Sum(nil)) {
		return legacyaudience.InboundWebhookIdentity{}, legacyaudience.ErrUnauthenticated
	}
	return legacyaudience.InboundWebhookIdentity{ClientID: clientID, TransportEventID: eventID}, nil
}

func singleAIAudienceHeader(request *http.Request, name string) (string, bool) {
	values := request.Header.Values(name)
	returnValue := ""
	if len(values) == 1 {
		returnValue = values[0]
	}
	return returnValue, len(values) == 1 && returnValue != "" && strings.TrimSpace(returnValue) == returnValue
}

func validAIAudienceProtocolEventID(value string) bool {
	if len(value) < 16 || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 0x21 || character > 0x7e {
			return false
		}
	}
	return true
}
