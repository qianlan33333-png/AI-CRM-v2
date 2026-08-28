package v1domain

import (
	"context"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type ServicePeriodHistoryJournal struct{ journals map[string]*Journal }

var servicePeriodHistoryScopes = map[string][2]string{
	"definitions":  {"public/service_period_products", "product_service_period_history"},
	"entitlements": {"public/service_period_entitlements", "product_service_period_entitlement_history"},
	"events":       {"public/service_period_events", "product_service_period_event_history"},
}

var _ productport.ServicePeriodHistoryJournal = (*ServicePeriodHistoryJournal)(nil)

func NewServicePeriodHistoryJournal(definitions, entitlements, events *Journal) (*ServicePeriodHistoryJournal, error) {
	if definitions == nil || entitlements == nil || events == nil {
		return nil, ErrInvalidScope
	}
	j := &ServicePeriodHistoryJournal{journals: map[string]*Journal{"definitions": definitions, "entitlements": entitlements, "events": events}}
	for kind := range servicePeriodHistoryScopes {
		selected, err := j.selectJournal(kind)
		if err != nil || selected.scope.ImportVersion != definitions.scope.ImportVersion || selected.scope.ArchiveRunID != definitions.scope.ArchiveRunID {
			return nil, ErrInvalidScope
		}
	}
	return j, nil
}

func (j *ServicePeriodHistoryJournal) selectJournal(kind string) (*Journal, error) {
	if j == nil {
		return nil, ErrInvalidScope
	}
	expected, ok := servicePeriodHistoryScopes[kind]
	selected := j.journals[kind]
	if !ok || selected == nil || selected.tx == nil || !selected.scope.valid() || selected.scope.AdapterID != v1archive.DefaultAdapterID ||
		selected.scope.TargetDomain != "product" || selected.scope.TableID != expected[0] || selected.scope.TargetTable != expected[1] {
		return nil, ErrInvalidScope
	}
	return selected, nil
}

func (j *ServicePeriodHistoryJournal) LoadServicePeriodHistory(ctx context.Context, kind, source string) (productport.ServicePeriodHistoryReceipt, bool, error) {
	selected, err := j.selectJournal(kind)
	if err != nil {
		return productport.ServicePeriodHistoryReceipt{}, false, err
	}
	terminal, found, err := selected.LoadTerminal(ctx, source)
	if err != nil || !found {
		return productport.ServicePeriodHistoryReceipt{}, found, err
	}
	receipt, err := servicePeriodHistoryReceipt(source, terminal)
	return receipt, err == nil, err
}

func servicePeriodHistoryReceipt(source string, terminal TerminalReceipt) (productport.ServicePeriodHistoryReceipt, error) {
	key, err := ParseSourceIdentifier(source)
	id, idErr := strconv.ParseInt(terminal.TargetID, 10, 64)
	if err != nil || idErr != nil || id < 1 || strconv.FormatInt(id, 10) != terminal.TargetID || terminal.SourceKeyDigest != key ||
		terminal.Disposition != "import" || terminal.Reason != "" || len(terminal.Metadata) != 0 || terminal.PayloadDigest == [32]byte{} || terminal.TargetDigest == [32]byte{} {
		return productport.ServicePeriodHistoryReceipt{}, ErrConflict
	}
	return productport.ServicePeriodHistoryReceipt{SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest}, nil
}

func (j *ServicePeriodHistoryJournal) RecordServicePeriodHistory(ctx context.Context, kind string, receipt productport.ServicePeriodHistoryReceipt) error {
	selected, err := j.selectJournal(kind)
	if err != nil {
		return err
	}
	key, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || key == [32]byte{} || receipt.PayloadDigest == [32]byte{} || receipt.TargetDigest == [32]byte{} || receipt.TargetID < 1 || receipt.Replayed {
		return ErrInvalidScope
	}
	return selected.Record(ctx, TerminalReceipt{SourceKeyDigest: key, PayloadDigest: receipt.PayloadDigest,
		Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest})
}
