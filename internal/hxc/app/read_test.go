package app

import (
	"context"
	"errors"
	"testing"
	"time"

	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

func TestReaderMergesEligibleStaffAndOrphansWithoutWrites(t *testing.T) {
	old := time.Date(2026, 8, 19, 1, 2, 3, 4, time.FixedZone("x", 8*3600))
	newer := old.Add(time.Hour)
	reader := Reader{Staff: staffStub{entries: []contact.StaffDirectoryEntry{{WeComUserID: "alice", DisplayName: "Alice", UpdatedAt: old}, {WeComUserID: "bob", DisplayName: "", UpdatedAt: newer}}}, Configs: configStub{configs: []port.SenderConfig{{ID: "b", SenderUserID: "bob", DisplayName: "Configured Bob", Priority: 3, IsActive: true}, {ID: "o", SenderUserID: "orphan", DisplayName: "", Priority: 2, IsActive: true}}}}
	got, err := reader.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.SendConfigs) != 2 || len(got.Directory) != 3 || got.ActiveSenderCount != 1 || !got.LastSyncedAt.Equal(newer.UTC()) {
		t.Fatalf("projection=%+v", got)
	}
	if got.Directory[0].WeComUserID != "alice" || got.Directory[0].IsSender || got.Directory[1].DisplayName != "Configured Bob" || !got.Directory[1].IsActive || got.Directory[2].WeComUserID != "orphan" || got.Directory[2].IsActive || got.Directory[2].DisplayName != "orphan" {
		t.Fatalf("directory=%+v", got.Directory)
	}
}

func TestReaderFailsClosed(t *testing.T) {
	if _, err := (Reader{}).Read(context.Background()); err == nil {
		t.Fatal("nil reader succeeded")
	}
	if _, err := (Reader{Staff: staffStub{err: errors.New("db")}, Configs: configStub{}}).Read(context.Background()); err == nil {
		t.Fatal("staff failure succeeded")
	}
	if _, err := (Reader{Staff: staffStub{}, Configs: configStub{err: errors.New("db")}}).Read(context.Background()); err == nil {
		t.Fatal("config failure succeeded")
	}
}

func TestReaderCanonicalizesStaffAndRejectsAmbiguousOrPaddedConfigIDs(t *testing.T) {
	now := time.Date(2026, 8, 19, 1, 2, 3, 0, time.UTC)
	reader := Reader{
		Staff: staffStub{entries: []contact.StaffDirectoryEntry{
			{WeComUserID: " zed ", DisplayName: " Zed ", UpdatedAt: now},
			{WeComUserID: "alice", DisplayName: "Alice", UpdatedAt: now},
		}},
		Configs: configStub{configs: []port.SenderConfig{
			{ID: "config-zed", SenderUserID: "zed", Priority: 7},
			{ID: "config-alice", SenderUserID: "alice", Priority: 7},
		}},
	}
	got, err := reader.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Directory[0].WeComUserID != "alice" || got.Directory[1].WeComUserID != "zed" || got.Directory[1].DisplayName != "Zed" ||
		got.SendConfigs[0].SenderUserID != "alice" || got.SendConfigs[1].SenderUserID != "zed" {
		t.Fatalf("projection is not canonically ordered: %#v", got)
	}

	cases := []struct {
		name   string
		staff  []contact.StaffDirectoryEntry
		config []port.SenderConfig
	}{
		{name: "staff canonical collision", staff: []contact.StaffDirectoryEntry{{WeComUserID: "alice"}, {WeComUserID: " alice "}}},
		{name: "padded config id", staff: []contact.StaffDirectoryEntry{{WeComUserID: "alice"}}, config: []port.SenderConfig{{ID: " config-alice ", SenderUserID: "alice"}}},
		{name: "padded config sender", staff: []contact.StaffDirectoryEntry{{WeComUserID: "alice"}}, config: []port.SenderConfig{{ID: "config-alice", SenderUserID: " alice "}}},
		{name: "duplicate canonical config sender", staff: []contact.StaffDirectoryEntry{{WeComUserID: "alice"}}, config: []port.SenderConfig{{ID: "config-a", SenderUserID: "alice"}, {ID: "config-b", SenderUserID: "alice"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reader := Reader{Staff: staffStub{entries: tc.staff}, Configs: configStub{configs: tc.config}}
			if _, err := reader.Read(context.Background()); !errors.Is(err, errInvalidProjection) {
				t.Fatalf("err=%v, want invalid projection", err)
			}
		})
	}
}

type staffStub struct {
	entries []contact.StaffDirectoryEntry
	err     error
}

func (s staffStub) ListEligibleStaff(context.Context) ([]contact.StaffDirectoryEntry, error) {
	return s.entries, s.err
}

type configStub struct {
	configs []port.SenderConfig
	err     error
}

func (s configStub) ListSenderConfigs(context.Context) ([]port.SenderConfig, error) {
	return s.configs, s.err
}
