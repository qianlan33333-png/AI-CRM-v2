package contactfixture

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateAttachmentReferenceRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	emptyPool := &pgxpool.Pool{}
	for _, test := range []struct {
		name         string
		pool         *pgxpool.Pool
		channelName  string
		code         string
		attachmentID int64
	}{
		{name: "nil pool", channelName: "channel", code: "channel-code", attachmentID: 1},
		{name: "empty name", pool: emptyPool, code: "channel-code", attachmentID: 1},
		{name: "empty code", pool: emptyPool, channelName: "channel", attachmentID: 1},
		{name: "non-positive attachment", pool: emptyPool, channelName: "channel", code: "channel-code"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := CreateAttachmentReference(context.Background(), test.pool, test.channelName, test.code, test.attachmentID); !errors.Is(err, ErrInvalidAttachmentReference) {
				t.Fatalf("CreateAttachmentReference error=%v, want ErrInvalidAttachmentReference", err)
			}
		})
	}
}
