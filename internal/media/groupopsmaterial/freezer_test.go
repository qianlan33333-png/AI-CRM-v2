package groupopsmaterial

import (
	"context"
	"errors"
	"testing"

	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

func TestFreezerReturnsValidatedImmutableProviderManifest(t *testing.T) {
	reader := &readerStub{value: PreparedPlan{
		Attachments: []mediaport.GroupOpsProviderReadyAttachment{
			{MsgType: "image", MediaID: "media-image-7"},
			{MsgType: "miniprogram", AppID: "wx-course", PagePath: "pages/today", Title: "今日课程", MediaID: "media-cover-7"},
			{MsgType: "file", MediaID: "media-file-7"},
			{MsgType: "link", Title: "加入体验群", URL: "https://work.weixin.qq.com/gm/0123456789abcdef0123456789abcdef", Description: "领取资料"},
		},
	}}
	freezer, err := NewFreezer(reader)
	if err != nil {
		t.Fatal(err)
	}
	plan := mediaport.GroupOpsMaterialPlan{References: []mediaport.GroupOpsMaterialReference{{Kind: "image", ID: 7}, {Kind: "miniprogram", ID: 8}, {Kind: "attachment", ID: 9}, {Kind: "group_invite", ID: 10}}}
	snapshot, err := freezer.FreezeGroupOpsMaterial(context.Background(), plan)
	if err != nil || reader.calls != 1 || len(snapshot.Attachments) != 4 {
		t.Fatalf("snapshot=%+v calls=%d err=%v", snapshot, reader.calls, err)
	}
	reader.value.Attachments[0].MediaID = "changed-after-freeze"
	if snapshot.Attachments[0].MediaID != "media-image-7" {
		t.Fatalf("snapshot aliases mutable reader data: %+v", snapshot)
	}
}

func TestFreezerFailsClosedForMismatchedOrUnreadyPackage(t *testing.T) {
	freezer, err := NewFreezer(&readerStub{err: errors.New("lease not ready")})
	if err != nil {
		t.Fatal(err)
	}
	validPlan := mediaport.GroupOpsMaterialPlan{References: []mediaport.GroupOpsMaterialReference{{Kind: "image", ID: 7}}}
	if _, err = freezer.FreezeGroupOpsMaterial(context.Background(), validPlan); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err=%v", err)
	}
	if _, err = freezer.FreezeGroupOpsMaterial(context.Background(), mediaport.GroupOpsMaterialPlan{References: []mediaport.GroupOpsMaterialReference{{Kind: "image", ID: 0}}}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err=%v", err)
	}
	mismatched, err := NewFreezer(&readerStub{value: PreparedPlan{Attachments: []mediaport.GroupOpsProviderReadyAttachment{{MsgType: "file", MediaID: "file-7"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mismatched.FreezeGroupOpsMaterial(context.Background(), validPlan); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("mismatched manifest err=%v", err)
	}
}

type readerStub struct {
	value PreparedPlan
	err   error
	calls int
}

func (stub *readerStub) ReadPreparedGroupOpsPlan(_ context.Context, _ mediaport.GroupOpsMaterialPlan) (PreparedPlan, error) {
	stub.calls++
	return stub.value, stub.err
}
