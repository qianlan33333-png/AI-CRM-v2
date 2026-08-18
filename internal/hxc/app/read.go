package app

import (
	"context"
	"errors"
	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	"sort"
	"strings"
	"time"
)

var errInvalidProjection = errors.New("invalid hxc sender projection")

type Reader struct {
	Staff   contact.StaffDirectoryReader
	Configs port.SenderConfigReader
}

type Candidate struct {
	WeComUserID string
	DisplayName string
	Position    string
	WeComStatus int
	IsSender    bool
	Priority    int
	IsActive    bool
}

type Projection struct {
	SendConfigs       []port.SenderConfig
	Directory         []Candidate
	ActiveSenderCount int
	LastSyncedAt      time.Time
}

func (r Reader) Read(ctx context.Context) (Projection, error) {
	if r.Staff == nil || r.Configs == nil {
		return Projection{}, errUnavailable
	}
	staff, err := r.Staff.ListEligibleStaff(ctx)
	if err != nil {
		return Projection{}, err
	}
	configs, err := r.Configs.ListSenderConfigs(ctx)
	if err != nil {
		return Projection{}, err
	}
	if staff, err = canonicalStaff(staff); err != nil {
		return Projection{}, err
	}
	if configs, err = canonicalConfigs(configs); err != nil {
		return Projection{}, err
	}
	projection := Projection{SendConfigs: configs, Directory: Merge(staff, configs)}
	for _, entry := range staff {
		if entry.UpdatedAt.After(projection.LastSyncedAt) {
			projection.LastSyncedAt = entry.UpdatedAt.UTC()
		}
	}
	for _, candidate := range projection.Directory {
		if candidate.IsSender && candidate.IsActive {
			projection.ActiveSenderCount++
		}
	}
	return projection, nil
}

func canonicalStaff(entries []contact.StaffDirectoryEntry) ([]contact.StaffDirectoryEntry, error) {
	seen := make(map[string]struct{}, len(entries))
	for i := range entries {
		entries[i].WeComUserID = strings.TrimSpace(entries[i].WeComUserID)
		entries[i].DisplayName = strings.TrimSpace(entries[i].DisplayName)
		if entries[i].WeComUserID == "" {
			return nil, errInvalidProjection
		}
		if _, duplicate := seen[entries[i].WeComUserID]; duplicate {
			return nil, errInvalidProjection
		}
		seen[entries[i].WeComUserID] = struct{}{}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].WeComUserID < entries[j].WeComUserID })
	return entries, nil
}

func canonicalConfigs(configs []port.SenderConfig) ([]port.SenderConfig, error) {
	ids := make(map[string]struct{}, len(configs))
	senders := make(map[string]struct{}, len(configs))
	for i := range configs {
		id := strings.TrimSpace(configs[i].ID)
		senderUserID := strings.TrimSpace(configs[i].SenderUserID)
		if id == "" || senderUserID == "" || configs[i].ID != id || configs[i].SenderUserID != senderUserID {
			return nil, errInvalidProjection
		}
		if _, duplicate := ids[id]; duplicate {
			return nil, errInvalidProjection
		}
		if _, duplicate := senders[senderUserID]; duplicate {
			return nil, errInvalidProjection
		}
		ids[id] = struct{}{}
		senders[senderUserID] = struct{}{}
		configs[i].DisplayName = strings.TrimSpace(configs[i].DisplayName)
	}
	sort.Slice(configs, func(i, j int) bool {
		return configs[i].Priority < configs[j].Priority || (configs[i].Priority == configs[j].Priority && configs[i].SenderUserID < configs[j].SenderUserID)
	})
	return configs, nil
}

func Merge(staff []contact.StaffDirectoryEntry, configs []port.SenderConfig) []Candidate {
	by := map[string]port.SenderConfig{}
	for _, c := range configs {
		by[c.SenderUserID] = c
	}
	out := make([]Candidate, 0, len(staff)+len(configs))
	for _, s := range staff {
		c, ok := by[s.WeComUserID]
		name := strings.TrimSpace(s.DisplayName)
		if ok && strings.TrimSpace(c.DisplayName) != "" {
			name = strings.TrimSpace(c.DisplayName)
		}
		if name == "" {
			name = s.WeComUserID
		}
		out = append(out, Candidate{WeComUserID: s.WeComUserID, DisplayName: name, IsSender: ok, Priority: c.Priority, IsActive: c.IsActive})
		delete(by, s.WeComUserID)
	}
	orphans := make([]Candidate, 0, len(by))
	for _, c := range by {
		name := strings.TrimSpace(c.DisplayName)
		if name == "" {
			name = c.SenderUserID
		}
		orphans = append(orphans, Candidate{WeComUserID: c.SenderUserID, DisplayName: name, IsSender: true, Priority: c.Priority, IsActive: false})
	}
	sort.Slice(orphans, func(i, j int) bool {
		return orphans[i].Priority < orphans[j].Priority || (orphans[i].Priority == orphans[j].Priority && orphans[i].WeComUserID < orphans[j].WeComUserID)
	})
	return append(out, orphans...)
}
