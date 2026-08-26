package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	groupopshttp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/http"
)

const groupOpsWebhookClientID = "aicrm-webhook-group-ops"

var groupOpsBroadcastExpectation = apiClientJWTExpectation{Audience: "external_integration", Purpose: "group_broadcast", Capability: "group_broadcast_execute", Scope: "write"}

// groupOpsProtocolReplay is the narrow durable boundary to be backed by the
// Group Ops protocol-receipt namespace during composition.
type groupOpsProtocolReplay interface {
	Reserve(context.Context, string, string, [32]byte) (bool, error)
}

type groupOpsProtocolAuthenticator struct {
	jwt        *apiClientJWTAuthenticator
	webhookKey []byte
	replay     groupOpsProtocolReplay
	now        func() time.Time
}

func (a *groupOpsProtocolAuthenticator) Authenticate(ctx context.Context, r *http.Request, purpose, resource string, body []byte) (groupopshttp.ProtocolPrincipal, error) {
	if a == nil || ctx == nil || r == nil || a.now == nil {
		return groupopshttp.ProtocolPrincipal{}, groupopshttp.ErrProtocolUnavailable
	}
	if purpose == "group_ops_broadcast" {
		if a.jwt == nil {
			return groupopshttp.ProtocolPrincipal{}, groupopshttp.ErrProtocolUnavailable
		}
		p, err := a.jwt.authenticate(ctx, r, groupOpsBroadcastExpectation)
		if err != nil {
			return groupopshttp.ProtocolPrincipal{}, err
		}
		return groupopshttp.ProtocolPrincipal{ID: p.ClientID}, nil
	}
	if purpose != "group_ops_webhook" || a.replay == nil || len(a.webhookKey) != sha256.Size || resource == "" {
		return groupopshttp.ProtocolPrincipal{}, groupopshttp.ErrProtocolUnavailable
	}
	client, timestamp, event, signature := r.Header.Get("X-AICRM-Client-Id"), r.Header.Get("X-AICRM-Timestamp"), r.Header.Get("X-AICRM-Event-Id"), r.Header.Get("X-AICRM-Signature")
	if client != groupOpsWebhookClientID || len(event) < 16 || len(event) > 256 || strings.TrimSpace(event) != event {
		return groupopshttp.ProtocolPrincipal{}, errors.New("unauthorized")
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || timestamp != strconv.FormatInt(ts, 10) || a.now().UTC().Sub(time.Unix(ts, 0).UTC()) > 5*time.Minute || time.Unix(ts, 0).UTC().Sub(a.now().UTC()) > time.Minute {
		return groupopshttp.ProtocolPrincipal{}, errors.New("unauthorized")
	}
	rawSig := strings.TrimPrefix(signature, "sha256=")
	decoded, err := hex.DecodeString(rawSig)
	if err != nil || len(decoded) != sha256.Size || rawSig != strings.ToLower(rawSig) {
		return groupopshttp.ProtocolPrincipal{}, errors.New("unauthorized")
	}
	mac := hmac.New(sha256.New, a.webhookKey)
	_, _ = mac.Write([]byte(timestamp + "\n" + event + "\n"))
	_, _ = mac.Write(body)
	if !hmac.Equal(decoded, mac.Sum(nil)) {
		return groupopshttp.ProtocolPrincipal{}, errors.New("unauthorized")
	}
	payloadBytes := make([]byte, 0, len(resource)+1+len(body))
	payloadBytes = append(payloadBytes, resource...)
	payloadBytes = append(payloadBytes, '\n')
	payloadBytes = append(payloadBytes, body...)
	payload := sha256.Sum256(payloadBytes)
	created, err := a.replay.Reserve(ctx, resource, event, payload)
	if err != nil {
		return groupopshttp.ProtocolPrincipal{}, groupopshttp.ErrProtocolUnavailable
	}
	if !created {
		return groupopshttp.ProtocolPrincipal{}, errors.New("unauthorized")
	}
	return groupopshttp.ProtocolPrincipal{ID: event}, nil
}
