package radarfixture

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateImageReferenceRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	emptyPool := &pgxpool.Pool{}
	for _, test := range []struct {
		name     string
		pool     *pgxpool.Pool
		code     string
		linkName string
		title    string
		imageID  int64
	}{
		{name: "nil pool", code: "rd_0123456789012345678901", linkName: "radar", title: "Radar", imageID: 1},
		{name: "empty code", pool: emptyPool, linkName: "radar", title: "Radar", imageID: 1},
		{name: "empty name", pool: emptyPool, code: "rd_0123456789012345678901", title: "Radar", imageID: 1},
		{name: "empty title", pool: emptyPool, code: "rd_0123456789012345678901", linkName: "radar", imageID: 1},
		{name: "zero image", pool: emptyPool, code: "rd_0123456789012345678901", linkName: "radar", title: "Radar"},
		{name: "negative image", pool: emptyPool, code: "rd_0123456789012345678901", linkName: "radar", title: "Radar", imageID: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := CreateImageReference(context.Background(), test.pool, test.code, test.linkName, test.title, test.imageID); !errors.Is(err, ErrInvalidImageReference) {
				t.Fatalf("CreateImageReference error=%v, want ErrInvalidImageReference", err)
			}
		})
	}
}
