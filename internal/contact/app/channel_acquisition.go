package app

import (
	"context"
	"encoding/json"
)

// ChannelAcquisitionPreview is the local, provider-free readiness projection
// for one channel. It neither creates a QR code nor dispatches an acquisition
// request; provider integration is deliberately outside this contract.
type ChannelAcquisitionPreview struct {
	ChannelID   int64                       `json:"channel_id"`
	ChannelCode string                      `json:"channel_code"`
	ChannelName string                      `json:"channel_name"`
	Assignees   []ChannelAssignee           `json:"assignees"`
	Lifecycle   ChannelAcquisitionLifecycle `json:"lifecycle"`
	QRCode      ChannelQRCodePreview        `json:"qrcode"`
	Share       ChannelSharePreview         `json:"share"`
}

type ChannelAcquisitionLifecycle struct {
	State             string   `json:"state"`
	EntrantReady      bool     `json:"entrant_ready"`
	ReadinessBlockers []string `json:"readiness_blockers"`
}

type ChannelQRCodePreview struct {
	Status     string `json:"status"`
	SceneValue string `json:"scene_value"`
	URL        string `json:"url"`
}

type ChannelSharePreview struct {
	URL      string `json:"url"`
	CopyText string `json:"copy_text"`
}

type channelAcquisitionReader interface {
	GetChannel(context.Context, int64) (Channel, error)
}

type ChannelAcquisitionService struct{ channels channelAcquisitionReader }

func NewChannelAcquisitionService(channels channelAcquisitionReader) *ChannelAcquisitionService {
	return &ChannelAcquisitionService{channels: channels}
}

func (service *ChannelAcquisitionService) Preview(ctx context.Context, channelID int64) (ChannelAcquisitionPreview, error) {
	if service == nil || service.channels == nil || channelID < 1 {
		return ChannelAcquisitionPreview{}, ErrInvalidChannel
	}
	channel, err := service.channels.GetChannel(ctx, channelID)
	if err != nil {
		return ChannelAcquisitionPreview{}, err
	}
	preview, err := channelAcquisitionPreview(channel)
	if err != nil {
		return ChannelAcquisitionPreview{}, err
	}
	return preview, nil
}

func channelAcquisitionPreview(channel Channel) (ChannelAcquisitionPreview, error) {
	if !validChannel(channel) {
		return ChannelAcquisitionPreview{}, ErrChannelUnavailable
	}
	values, err := object(channel.LegacyProjection)
	if err != nil {
		return ChannelAcquisitionPreview{}, ErrChannelUnavailable
	}
	var channelType, carrierType, sceneValue, qrURL, linkURL, finalURL string
	for key, target := range map[string]*string{
		"channel_type": &channelType, "carrier_type": &carrierType, "scene_value": &sceneValue,
		"qr_url": &qrURL, "link_url": &linkURL, "final_url": &finalURL,
	} {
		if json.Unmarshal(values[key], target) != nil {
			return ChannelAcquisitionPreview{}, ErrChannelUnavailable
		}
	}
	if channelType == "wecom_customer_acquisition" || carrierType == "link" {
		carrierType = "link"
	} else {
		carrierType = "qrcode"
	}
	shareURL := finalURL
	if shareURL == "" {
		shareURL = linkURL
	}
	qrStatus := "not_generated"
	if qrURL != "" && sceneValue != "" {
		qrStatus = "legacy_untracked"
	}
	activeAssignees := make([]ChannelAssignee, 0, len(channel.Assignees))
	for _, assignee := range channel.Assignees {
		if assignee.Status == "active" {
			activeAssignees = append(activeAssignees, assignee)
		}
	}
	blockers := make([]string, 0, 4)
	state := "draft"
	if channel.Status == "archived" {
		state, blockers = "archived", append(blockers, "channel_archived")
	} else if channel.Status == "inactive" {
		state, blockers = "paused", append(blockers, "channel_inactive")
	} else {
		if len(activeAssignees) == 0 {
			blockers = append(blockers, "active_assignee_required")
		}
		if carrierType == "qrcode" {
			if sceneValue == "" {
				blockers = append(blockers, "scene_value_required")
			}
			if qrURL == "" {
				blockers = append(blockers, "qrcode_required")
			}
		} else if shareURL == "" {
			blockers = append(blockers, "share_url_required")
		}
		if len(blockers) == 0 {
			state = "local_prerequisites_ready"
		}
		// QR and share values in this local projection have no provider asset
		// receipt. They are useful to inspect, but cannot authorize entrants.
		blockers = append(blockers, "provider_asset_unverified")
	}
	return ChannelAcquisitionPreview{
		ChannelID: channel.ID, ChannelCode: channel.ChannelCode, ChannelName: channel.ChannelName,
		Assignees: activeAssignees,
		Lifecycle: ChannelAcquisitionLifecycle{State: state, EntrantReady: false, ReadinessBlockers: blockers},
		QRCode:    ChannelQRCodePreview{Status: qrStatus, SceneValue: sceneValue, URL: qrURL},
		Share:     ChannelSharePreview{URL: shareURL, CopyText: firstChannelAcquisitionValue(shareURL, qrURL)},
	}, nil
}

func firstChannelAcquisitionValue(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
