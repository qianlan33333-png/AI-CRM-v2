package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaximumRemarkRunes   = 500
	MaximumAllianceRunes = 120
)

type State string

const (
	StateActive  State = "active"
	StateExpired State = "expired"
	StateRemoved State = "removed"
)

func (state State) Valid() bool {
	return state == StateActive || state == StateExpired || state == StateRemoved
}

type Source string

const (
	SourceManual    Source = "manual"
	SourcePaidOrder Source = "paid_order"
)

func (source Source) Valid() bool {
	return source == SourceManual || source == SourcePaidOrder
}

type Member struct {
	MemberRef        string     `json:"member_ref"`
	ServiceProductID int64      `json:"service_product_id"`
	CustomerID       int64      `json:"customer_id"`
	State            State      `json:"state"`
	Source           Source     `json:"source"`
	StartsAt         time.Time  `json:"starts_at"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	ExpiredAt        *time.Time `json:"expired_at,omitempty"`
	RemovedAt        *time.Time `json:"removed_at,omitempty"`
	Remark           *string    `json:"remark,omitempty"`
	Alliance         *string    `json:"alliance,omitempty"`
	Version          int64      `json:"version"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (member Member) Valid() bool {
	if !ValidMemberRef(member.MemberRef) || member.ServiceProductID < 1 || member.CustomerID < 1 ||
		!member.State.Valid() || !member.Source.Valid() || member.Version < 1 || member.StartsAt.IsZero() ||
		member.CreatedAt.IsZero() || member.UpdatedAt.Before(member.CreatedAt) ||
		!ValidOptionalText(member.Remark, MaximumRemarkRunes) || !ValidOptionalText(member.Alliance, MaximumAllianceRunes) {
		return false
	}
	if member.ExpiresAt != nil && (member.ExpiresAt.IsZero() || member.ExpiresAt.Before(member.StartsAt)) {
		return false
	}
	switch member.State {
	case StateActive:
		return member.ExpiredAt == nil && member.RemovedAt == nil
	case StateExpired:
		return validTransitionTime(member.ExpiredAt, member.StartsAt) && member.RemovedAt == nil
	case StateRemoved:
		return validTransitionTime(member.RemovedAt, member.StartsAt)
	default:
		return false
	}
}

func ValidMemberRef(value string) bool {
	if len(value) != 26 || !strings.HasPrefix(value, "spm_") {
		return false
	}
	for _, character := range value[4:] {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_') {
			return false
		}
	}
	return true
}

func ValidOptionalText(value *string, maximum int) bool {
	if value == nil {
		return true
	}
	return *value != "" && strings.TrimSpace(*value) == *value && utf8.ValidString(*value) &&
		utf8.RuneCountInString(*value) <= maximum
}

func validTransitionTime(value *time.Time, startsAt time.Time) bool {
	return value != nil && !value.IsZero() && !value.Before(startsAt)
}
