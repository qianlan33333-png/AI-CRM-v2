// Package groupopsmaterial freezes Media-owned, provider-ready package facts
// for Group Ops acceptance. It deliberately does not call WeCom: the source
// must have completed the Media-owned preparation/lease boundary before this
// method returns a snapshot that an outbound worker can submit unchanged.
package groupopsmaterial

import (
	"context"
	"errors"

	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

var ErrUnavailable = errors.New("group ops material freezer unavailable")

type PreparedPlan struct {
	Attachments []mediaport.GroupOpsProviderReadyAttachment
}

// PreparedPlanReader is implemented inside Media's composition root. Its read
// must lock every ordered local reference and only return media IDs backed by
// ready Media prep/lease receipts. It must never expose a URL/blob reference
// to the Group Ops worker.
type PreparedPlanReader interface {
	ReadPreparedGroupOpsPlan(context.Context, mediaport.GroupOpsMaterialPlan) (PreparedPlan, error)
}

type Freezer struct{ reader PreparedPlanReader }

var _ mediaport.GroupOpsMaterialSnapshotFreezer = (*Freezer)(nil)

func NewFreezer(reader PreparedPlanReader) (*Freezer, error) {
	if reader == nil {
		return nil, ErrUnavailable
	}
	return &Freezer{reader: reader}, nil
}

func (freezer *Freezer) FreezeGroupOpsMaterial(ctx context.Context, plan mediaport.GroupOpsMaterialPlan) (mediaport.GroupOpsMaterialSnapshot, error) {
	if freezer == nil || freezer.reader == nil || ctx == nil || mediaport.ValidateGroupOpsMaterialPlan(plan) != nil {
		return mediaport.GroupOpsMaterialSnapshot{}, ErrUnavailable
	}
	pkg, err := freezer.reader.ReadPreparedGroupOpsPlan(ctx, plan)
	if err != nil || !matchesPlan(plan, pkg.Attachments) {
		return mediaport.GroupOpsMaterialSnapshot{}, ErrUnavailable
	}
	snapshot := mediaport.GroupOpsMaterialSnapshot{
		SchemaVersion: 2, NodeKind: "message",
		Attachments: append([]mediaport.GroupOpsProviderReadyAttachment(nil), pkg.Attachments...),
	}
	if err := mediaport.ValidateGroupOpsMaterialSnapshot(snapshot); err != nil {
		return mediaport.GroupOpsMaterialSnapshot{}, ErrUnavailable
	}
	return snapshot, nil
}

func matchesPlan(plan mediaport.GroupOpsMaterialPlan, attachments []mediaport.GroupOpsProviderReadyAttachment) bool {
	if len(plan.References) != len(attachments) {
		return false
	}
	for index, reference := range plan.References {
		want := reference.Kind
		if want == "attachment" {
			want = "file"
		}
		if want == "group_invite" {
			want = "link"
		}
		if attachments[index].MsgType != want {
			return false
		}
	}
	return true
}
