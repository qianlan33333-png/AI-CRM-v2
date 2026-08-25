package worker

import (
	"errors"
	"testing"

	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	"github.com/riverqueue/river"
)

func TestInboundIdentityPendingUsesControlledRiverSnooze(t *testing.T) {
	err := inboundWorkResult(wecomapp.ErrInboundIdentityPending)
	var snooze *river.JobSnoozeError
	if !errors.As(err, &snooze) || snooze.Duration != wecomapp.InboundIdentityRetryPeriod {
		t.Fatalf("snooze = %#v", err)
	}
	if got := inboundWorkResult(errors.New("storage failed")); got == nil || errors.As(got, &snooze) {
		t.Fatalf("ordinary failure was changed to snooze: %v", got)
	}
}
