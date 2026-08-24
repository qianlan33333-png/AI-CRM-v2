package radarfixture

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
		code         string
		linkName     string
		title        string
		attachmentID int64
	}{
		{name: "nil pool", code: "rd_0123456789012345678901", linkName: "radar", title: "Radar", attachmentID: 1},
		{name: "empty code", pool: emptyPool, linkName: "radar", title: "Radar", attachmentID: 1},
		{name: "empty name", pool: emptyPool, code: "rd_0123456789012345678901", title: "Radar", attachmentID: 1},
		{name: "empty title", pool: emptyPool, code: "rd_0123456789012345678901", linkName: "radar", attachmentID: 1},
		{name: "zero attachment", pool: emptyPool, code: "rd_0123456789012345678901", linkName: "radar", title: "Radar"},
		{name: "negative attachment", pool: emptyPool, code: "rd_0123456789012345678901", linkName: "radar", title: "Radar", attachmentID: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := CreateAttachmentReference(context.Background(), test.pool, test.code, test.linkName, test.title, test.attachmentID); !errors.Is(err, ErrInvalidAttachmentReference) {
				t.Fatalf("CreateAttachmentReference error=%v, want ErrInvalidAttachmentReference", err)
			}
		})
	}
}
