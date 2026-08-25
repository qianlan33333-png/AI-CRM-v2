package app

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCH01AcquisitionPreviewDerivesLocalReadinessWithoutProviderExecution(t *testing.T) {
	service, _, _ := channelTestService()
	created, err := service.CreateChannel(context.Background(), CreateChannelCommand{
		Actor: 7, IdempotencyKey: "channel-preview-key-0001", ChannelName: "公开课",
		LegacyProjection: json.RawMessage(`{"owner_staff_id":"staff-7","scene_value":"scene-7","qr_url":"https://cdn.example.test/channel-7.jpg","final_url":"https://go.example.test/channel-7"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := NewChannelAcquisitionService(service).Preview(context.Background(), created.ID)
	if err != nil || preview.Lifecycle.EntrantReady || preview.Lifecycle.State != "local_prerequisites_ready" || len(preview.Lifecycle.ReadinessBlockers) != 1 || preview.Lifecycle.ReadinessBlockers[0] != "provider_asset_unverified" || preview.QRCode.Status != "legacy_untracked" || preview.Share.URL != "https://go.example.test/channel-7" || preview.Share.CopyText != preview.Share.URL || len(preview.Assignees) != 1 {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	if preview.QRCode.URL != "https://cdn.example.test/channel-7.jpg" || preview.Assignees[0].DisplayName != "成员 7" {
		t.Fatalf("preview=%+v", preview)
	}
}

func TestCH01AcquisitionPreviewMasksMissingLocalPrerequisitesAsDraft(t *testing.T) {
	service, _, _ := channelTestService()
	created, err := service.CreateChannel(context.Background(), CreateChannelCommand{
		Actor: 7, IdempotencyKey: "channel-preview-key-0002", ChannelName: "待生成二维码",
		LegacyProjection: json.RawMessage(`{"owner_staff_id":"staff-7","scene_value":"scene-7"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := NewChannelAcquisitionService(service).Preview(context.Background(), created.ID)
	if err != nil || preview.Lifecycle.EntrantReady || preview.Lifecycle.State != "draft" || len(preview.Lifecycle.ReadinessBlockers) != 2 || preview.Lifecycle.ReadinessBlockers[0] != "qrcode_required" || preview.Lifecycle.ReadinessBlockers[1] != "provider_asset_unverified" || preview.QRCode.Status != "not_generated" {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
}

func TestCH01AcquisitionPreviewSupportsLinkShareCarrier(t *testing.T) {
	service, _, _ := channelTestService()
	created, err := service.CreateChannel(context.Background(), CreateChannelCommand{
		Actor: 7, IdempotencyKey: "channel-preview-key-0003", ChannelName: "企微获客链接",
		LegacyProjection: json.RawMessage(`{"owner_staff_id":"staff-7","channel_type":"wecom_customer_acquisition","carrier_type":"link","link_url":"https://go.example.test/link-7"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := NewChannelAcquisitionService(service).Preview(context.Background(), created.ID)
	if err != nil || preview.Lifecycle.State != "local_prerequisites_ready" || preview.Lifecycle.EntrantReady || len(preview.Lifecycle.ReadinessBlockers) != 1 || preview.Lifecycle.ReadinessBlockers[0] != "provider_asset_unverified" || preview.Share.URL != "https://go.example.test/link-7" || preview.Share.CopyText != preview.Share.URL || preview.QRCode.Status != "not_generated" {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
}
