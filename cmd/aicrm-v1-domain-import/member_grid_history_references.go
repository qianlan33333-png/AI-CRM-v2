package main

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store"
)

type memberGridHistoryDefinitionReader interface {
	GetServicePeriodHistoryDefinition(context.Context, int64) (productport.ServicePeriodHistoryDefinition, error)
}

type memberGridHistoryReferences struct {
	customer    *channelCustomerResolver
	definitions map[int64]v1archive.ArchivedRow
	journal     *v1domain.Journal
	reader      memberGridHistoryDefinitionReader
}

// The caller first reconciles the prior service-period package. Missing source
// links can then remain NULL without concealing an unfinished prerequisite.
func newMemberGridHistoryReferences(ctx context.Context, archive v1domain.ArchiveSource, uow *platformstore.UnitOfWork, run string, dm01Run int64, key []byte) (*memberGridHistoryReferences, error) {
	customer, err := newChannelCustomerResolver(ctx, uow, dm01Run, key)
	if err != nil {
		return nil, err
	}
	journal, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: servicePeriodImportVersion, ArchiveRunID: run,
		AdapterID: v1archive.DefaultAdapterID, TableID: "public/service_period_products", TargetDomain: "product", TargetTable: "product_service_period_history"})
	if err != nil {
		return nil, err
	}
	r := &memberGridHistoryReferences{customer: customer, journal: journal, definitions: map[int64]v1archive.ArchivedRow{}, reader: productstore.NewServicePeriodHistoryStore()}
	err = archive.EachTableRow(ctx, run, "public/service_period_products", func(row v1archive.ArchivedRow) error {
		var fields map[string]json.RawMessage
		var id int64
		if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != "public/service_period_products" || v1archive.IsRedacted(row, "id") ||
			json.Unmarshal(row.Payload, &fields) != nil || json.Unmarshal(fields["id"], &id) != nil || id < 1 {
			return v1domain.ErrConflict
		}
		if _, exists := r.definitions[id]; exists {
			return v1domain.ErrConflict
		}
		r.definitions[id] = row
		return nil
	})
	return r, err
}

func (r *memberGridHistoryReferences) ResolveHistoricalMemberGridCustomer(ctx context.Context, unionID string) (*int64, error) {
	if r == nil || r.customer == nil {
		return nil, v1domain.ErrInvalidScope
	}
	return r.customer.ResolveHistoricalChannelCustomer(ctx, unionID)
}

func (r *memberGridHistoryReferences) ResolveHistoricalMemberGridProduct(ctx context.Context, sourceID int64) (*int64, error) {
	if r == nil || r.journal == nil || r.reader == nil || sourceID < 1 {
		return nil, v1domain.ErrInvalidScope
	}
	source, found := r.definitions[sourceID]
	if !found {
		return nil, nil
	}
	receipt, found, err := r.journal.LoadTerminal(ctx, v1domain.SourceIdentifier(source.SourceKeyHMAC))
	if err != nil {
		return nil, err
	}
	if !found || receipt.PayloadDigest != source.PayloadHMAC {
		return nil, v1domain.ErrConflict
	}
	if receipt.Disposition != "import" {
		return nil, nil
	}
	id, err := strconv.ParseInt(receipt.TargetID, 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != receipt.TargetID {
		return nil, v1domain.ErrConflict
	}
	actual, err := r.reader.GetServicePeriodHistoryDefinition(ctx, id)
	if err != nil {
		return nil, err
	}
	if !memberGridHistoryProductMatches(actual, sourceID, receipt) {
		return nil, v1domain.ErrConflict
	}
	return &actual.ProductID, nil
}

func memberGridHistoryProductMatches(actual productport.ServicePeriodHistoryDefinition, sourceID int64, receipt v1domain.TerminalReceipt) bool {
	return actual.ID > 0 && strconv.FormatInt(actual.ID, 10) == receipt.TargetID && actual.SourceDefinitionID == sourceID && actual.ProductID > 0 &&
		!actual.CreatedAt.IsZero() && !actual.UpdatedAt.IsZero() && !actual.UpdatedAt.Before(actual.CreatedAt) &&
		productapp.ServicePeriodHistoryDefinitionTargetDigest(actual) == receipt.TargetDigest
}
