package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1radarhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

const radarClickHistoryLinkTable = "public/radar_links"

// newRadarClickHistoryJournal owns one inert receipt stream; it never calls a
// current Radar action, tracking endpoint, or Provider.
func newRadarClickHistoryJournal(run string) (v1domain.RadarClickHistoryImportJournal, error) {
	terminal, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: v1domain.RadarClickHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: v1radarhistory.ClickTableID, TargetDomain: "radar", TargetTable: v1domain.RadarClickHistoryTarget})
	if err != nil {
		return nil, err
	}
	return v1domain.NewRadarClickHistoryJournal(terminal)
}

type radarClickHistoryReferences struct {
	customers *channelCustomerResolver
	links     radarport.Repository
	journal   *v1domain.Journal
	sourceKey []byte
}

func newRadarClickHistoryReferences(ctx context.Context, uow *platformstore.UnitOfWork, archiveRun string, dm01Run int64, dm01Key, archiveSourceKey []byte, links radarport.Repository) (*radarClickHistoryReferences, error) {
	if ctx == nil || uow == nil || archiveRun == "" || len(archiveSourceKey) < sha256.Size || links == nil {
		return nil, v1domain.ErrInvalidScope
	}
	customers, err := newChannelCustomerResolver(ctx, uow, dm01Run, dm01Key)
	if err != nil {
		return nil, err
	}
	journal, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: domainImportVersion, ArchiveRunID: archiveRun, AdapterID: v1archive.DefaultAdapterID, TableID: radarClickHistoryLinkTable, TargetDomain: "radar", TargetTable: "radar_links"})
	if err != nil {
		return nil, err
	}
	return &radarClickHistoryReferences{customers: customers, links: links, journal: journal, sourceKey: append([]byte(nil), archiveSourceKey...)}, nil
}

func (r *radarClickHistoryReferences) ResolveHistoricalRadarClick(ctx context.Context, fact v1radarhistory.ClickFact) (*int64, *int64, error) {
	if r == nil || r.customers == nil || r.links == nil || r.journal == nil || len(r.sourceKey) < sha256.Size || fact.LinkSourceID < 1 {
		return nil, nil, v1domain.ErrInvalidScope
	}
	radarLinkID, err := r.radarLink(ctx, fact.LinkSourceID)
	if err != nil {
		return nil, nil, err
	}
	customerID, err := r.customers.ResolveHistoricalChannelCustomer(ctx, fact.Source.UnionID)
	if err != nil {
		return nil, nil, err
	}
	return radarLinkID, customerID, nil
}

func (r *radarClickHistoryReferences) radarLink(ctx context.Context, sourceID int64) (*int64, error) {
	keyJSON, err := json.Marshal([]int64{sourceID})
	if err != nil {
		return nil, v1domain.ErrConflict
	}
	key, err := v1archive.SourceKeyHMAC(r.sourceKey, "radar_links", keyJSON)
	if err != nil {
		return nil, v1domain.ErrConflict
	}
	receipt, found, err := r.journal.LoadTerminal(ctx, v1domain.SourceIdentifier(key))
	if err != nil || !found {
		return nil, err
	}
	if receipt.Disposition != "import" {
		return nil, nil
	}
	id, err := strconv.ParseInt(receipt.TargetID, 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != receipt.TargetID {
		return nil, v1domain.ErrConflict
	}
	link, err := r.links.Get(ctx, radarport.LinkID(id))
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(link)
	if err != nil || sha256.Sum256(encoded) != receipt.TargetDigest || int64(link.LinkID) != id {
		return nil, v1domain.ErrConflict
	}
	return &id, nil
}
