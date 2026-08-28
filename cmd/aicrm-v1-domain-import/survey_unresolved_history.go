package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
	surveystore "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/store"
)

type surveyUnresolvedReferences struct {
	run            string
	questionnaires map[int64]v1archive.ArchivedRow
	journal        *v1domain.Journal
	customers      *channelCustomerResolver
}

func (r *surveyUnresolvedReferences) VerifySurveyUnresolvedSource(ctx context.Context, row v1archive.ArchivedRow) error {
	return v1domain.VerifySurveyUnresolvedSource(ctx, r.run, row)
}
func (r *surveyUnresolvedReferences) ResolveSurveyUnresolvedCustomer(ctx context.Context, unionID string) (*int64, error) {
	return r.customers.ResolveHistoricalChannelCustomer(ctx, unionID)
}
func (r *surveyUnresolvedReferences) ResolveSurveyUnresolvedQuestionnaire(ctx context.Context, sourceID int64) (*int64, error) {
	row, found := r.questionnaires[sourceID]
	if !found {
		return nil, nil
	}
	receipt, found, err := r.journal.LoadTerminal(ctx, v1domain.SourceIdentifier(row.SourceKeyHMAC))
	if err != nil {
		return nil, err
	}
	if !found || receipt.Disposition != "import" || receipt.PayloadDigest != row.PayloadHMAC {
		return nil, v1domain.ErrConflict
	}
	id, err := strconv.ParseInt(receipt.TargetID, 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != receipt.TargetID {
		return nil, v1domain.ErrConflict
	}
	digest := sha256.Sum256([]byte(row.TableID + "\x00" + receipt.TargetID + "\x00" + hex.EncodeToString(row.PayloadHMAC[:])))
	if digest != receipt.TargetDigest {
		return nil, v1domain.ErrConflict
	}
	if _, err = surveystore.NewQuestionnaireRepository().Get(ctx, surveyport.ID(id)); err != nil {
		return nil, err
	}
	return &id, nil
}

func importSurveyUnresolvedHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string, dm01Run int64, dm01Key, archiveSourceKey []byte) (v1domain.SurveyUnresolvedHistoryImportResult, error) {
	var result v1domain.SurveyUnresolvedHistoryImportResult
	customers, err := newChannelCustomerResolver(ctx, uow, dm01Run, dm01Key)
	if err != nil {
		return result, err
	}
	oldJournal, err := newJournal(run, "public/questionnaires", "survey", "questionnaires")
	if err != nil {
		return result, err
	}
	refs := &surveyUnresolvedReferences{run: run, questionnaires: map[int64]v1archive.ArchivedRow{}, journal: oldJournal, customers: customers}
	err = archive.EachTableRow(ctx, run, "public/questionnaires", func(row v1archive.ArchivedRow) error {
		var source struct {
			ID int64 `json:"id"`
		}
		if json.Unmarshal(row.Payload, &source) != nil || source.ID < 1 {
			return v1domain.ErrConflict
		}
		if _, exists := refs.questionnaires[source.ID]; exists {
			return v1domain.ErrConflict
		}
		refs.questionnaires[source.ID] = row
		return nil
	})
	if err != nil {
		return result, err
	}
	journal, err := v1domain.NewSurveyUnresolvedHistoryJournal(run)
	if err != nil {
		return result, err
	}
	writer, err := surveyapp.NewSurveyUnresolvedHistoryWriter(surveystore.NewSurveyUnresolvedHistoryStore(), journal)
	if err != nil {
		return result, err
	}
	importer, err := v1domain.NewSurveyUnresolvedHistoryImporter(archive, uow, writer, refs, archiveSourceKey)
	if err != nil {
		return result, err
	}
	return importer.Import(ctx, run)
}
