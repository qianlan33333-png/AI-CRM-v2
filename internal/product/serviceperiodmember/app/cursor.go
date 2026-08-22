package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	memberdomain "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/domain"
	memberport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/port"
)

type CursorCodec struct {
	secret []byte
}

type cursorPayload struct {
	Version   int                  `json:"v"`
	ProductID int64                `json:"product_id"`
	State     *memberdomain.State  `json:"state,omitempty"`
	Source    *memberdomain.Source `json:"source,omitempty"`
	UpdatedAt string               `json:"updated_at"`
	MemberRef string               `json:"member_ref"`
}

func NewCursorCodec(secret []byte) (*CursorCodec, error) {
	if len(secret) < 32 {
		return nil, memberport.ErrUnavailable
	}
	return &CursorCodec{secret: append([]byte(nil), secret...)}, nil
}

func (codec *CursorCodec) Encode(filter memberport.Filter, position memberport.Position) (string, error) {
	if codec == nil || len(codec.secret) < 32 || !validFilter(filter) || position.UpdatedAt.IsZero() || !memberdomain.ValidMemberRef(position.MemberRef) {
		return "", memberport.ErrUnavailable
	}
	payload := cursorPayload{Version: 1, ProductID: filter.ServiceProductID, State: cloneState(filter.State), Source: cloneSource(filter.Source), UpdatedAt: position.UpdatedAt.UTC().Format(time.RFC3339Nano), MemberRef: position.MemberRef}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", memberport.ErrUnavailable
	}
	signature := codec.sign(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (codec *CursorCodec) Decode(value string, filter memberport.Filter) (memberport.Position, error) {
	if codec == nil || len(codec.secret) < 32 || !validFilter(filter) || len(value) > 1024 {
		return memberport.Position{}, memberport.ErrInvalidInput
	}
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return memberport.Position{}, memberport.ErrInvalidInput
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return memberport.Position{}, memberport.ErrInvalidInput
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, codec.sign(raw)) {
		return memberport.Position{}, memberport.ErrInvalidInput
	}
	var payload cursorPayload
	if json.Unmarshal(raw, &payload) != nil || payload.Version != 1 || payload.ProductID != filter.ServiceProductID ||
		!optionalStateEqual(payload.State, filter.State) || !optionalSourceEqual(payload.Source, filter.Source) ||
		!memberdomain.ValidMemberRef(payload.MemberRef) {
		return memberport.Position{}, memberport.ErrInvalidInput
	}
	canonical, err := json.Marshal(payload)
	if err != nil || !hmac.Equal(canonical, raw) {
		return memberport.Position{}, memberport.ErrInvalidInput
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, payload.UpdatedAt)
	if err != nil || updatedAt.IsZero() {
		return memberport.Position{}, memberport.ErrInvalidInput
	}
	return memberport.Position{UpdatedAt: updatedAt.UTC(), MemberRef: payload.MemberRef}, nil
}

func (codec *CursorCodec) sign(raw []byte) []byte {
	mac := hmac.New(sha256.New, codec.secret)
	_, _ = mac.Write(raw)
	return mac.Sum(nil)
}

func cloneState(value *memberdomain.State) *memberdomain.State {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneSource(value *memberdomain.Source) *memberdomain.Source {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func optionalStateEqual(left, right *memberdomain.State) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func optionalSourceEqual(left, right *memberdomain.Source) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
