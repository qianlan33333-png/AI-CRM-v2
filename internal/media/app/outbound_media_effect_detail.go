package app

import (
	"context"
	"errors"
	"strings"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var ErrOutboundMediaEffectDetailInvalid = errors.New("invalid outbound media effect detail")

type OutboundMediaEffectDetail struct {
	ContentPackageID int64  `json:"content_package_id"`
	EffectID         string `json:"effect_id"`
	State            string `json:"state"`
	ProviderAccepted bool   `json:"provider_accepted"`
	DeliveryProven   bool   `json:"delivery_proven"`
}

type OutboundMediaEffectDetailReader interface {
	ReadOutboundMediaEffectDetail(context.Context, int64, string) (OutboundMediaEffectDetail, error)
}

type OutboundMediaEffectDetailService struct {
	uow    platformport.UnitOfWork
	reader OutboundMediaEffectDetailReader
}

func NewOutboundMediaEffectDetailService(uow platformport.UnitOfWork, reader OutboundMediaEffectDetailReader) *OutboundMediaEffectDetailService {
	return &OutboundMediaEffectDetailService{uow: uow, reader: reader}
}

func (s *OutboundMediaEffectDetailService) ReadOutboundMediaEffectDetail(ctx context.Context, contentPackageID int64, targetRef string) (OutboundMediaEffectDetail, error) {
	if s == nil || s.uow == nil || s.reader == nil || ctx == nil || contentPackageID < 1 || strings.TrimSpace(targetRef) == "" {
		return OutboundMediaEffectDetail{}, ErrOutboundMediaEffectDetailInvalid
	}
	var detail OutboundMediaEffectDetail
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var err error
		detail, err = s.reader.ReadOutboundMediaEffectDetail(tx, contentPackageID, mediaEERDigest("outbound-media-target", targetRef))
		return err
	})
	return detail, err
}
