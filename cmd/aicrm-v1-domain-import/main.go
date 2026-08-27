// Command aicrm-v1-domain-import materializes explicitly approved V1 archive
// rows through V2 domain-owned writers. It never connects to or mutates V1.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1candidate"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignstore "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	media "github.com/qianlan33333-png/AI-CRM-v2/internal/media"
	mediastore "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	orderstore "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	radarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/app"
	radarstore "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/store"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveystore "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/store"
)

const domainImportVersion = "v1-domain-a1"

func main() {
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	if err := run(os.Args[1:], environment); err != nil {
		fmt.Fprintln(os.Stderr, "aicrm-v1-domain-import:", err)
		os.Exit(2)
	}
}

func run(args []string, environment appconfig.V1ArchiveRuntime) error {
	flags := flag.NewFlagSet("aicrm-v1-domain-import", flag.ContinueOnError)
	mode := flags.String("mode", "import", "import|reconcile")
	domain := flags.String("domain", "", "campaign|survey|media|radar|shop|all")
	archiveRunID := flags.String("archive-run-id", "", "reconciled V1 archive run")
	actorValues := flags.String("campaign-actors", "", "explicit owner_userid=V2_actor_id pairs")
	migrationActor := flags.Int64("migration-actor", 0, "explicit V2 actor for local historical definitions")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if (*mode != "import" && *mode != "reconcile") || !validDomain(*domain) || *archiveRunID == "" || environment.TargetDatabaseURL == "" {
		return fmt.Errorf("valid mode, domain, archive-run-id and target database are required")
	}
	if *mode == "import" && len(environment.ArchiveKey) != 32 {
		return fmt.Errorf("32-byte archive key is required for import")
	}
	if *mode == "reconcile" && *domain != "all" {
		return fmt.Errorf("reconcile requires domain=all")
	}
	var actors v1candidate.ActorIDs
	var err error
	if *mode == "import" && (*domain == "campaign" || *domain == "all") {
		actors, err = parseActorIDs(*actorValues)
		if err != nil {
			return err
		}
	}
	if *mode == "import" && (*domain == "survey" || *domain == "media" || *domain == "radar" || *domain == "all") && *migrationActor < 1 {
		return fmt.Errorf("migration-actor is required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, environment.TargetDatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if *mode == "reconcile" {
		result, err := v1domain.ReconcileAll(ctx, pool, domainImportVersion, *archiveRunID)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"reconciliation": result})
	}
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		return err
	}
	defer archive.Close()
	uow := platformstore.NewUnitOfWork(pool)
	result := map[string]any{}
	if *domain == "campaign" || *domain == "all" {
		value, err := importCampaign(ctx, archive, uow, *archiveRunID, actors)
		if err != nil {
			return err
		}
		result["campaign"] = value
	}
	if *domain == "survey" || *domain == "all" {
		value, err := importSurvey(ctx, archive, uow, *archiveRunID, *migrationActor)
		if err != nil {
			return err
		}
		result["survey"] = value
	}
	if *domain == "media" || *domain == "all" {
		value, err := importMedia(ctx, archive, uow, *archiveRunID, *migrationActor)
		if err != nil {
			return err
		}
		result["media"] = value
	}
	if *domain == "radar" || *domain == "all" {
		value, err := importRadar(ctx, archive, uow, *archiveRunID, *migrationActor)
		if err != nil {
			return err
		}
		result["radar"] = value
	}
	if *domain == "shop" || *domain == "all" {
		value, err := importShop(ctx, archive, uow, *archiveRunID)
		if err != nil {
			return err
		}
		result["shop"] = value
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func validDomain(value string) bool {
	return value == "campaign" || value == "survey" || value == "media" || value == "radar" || value == "shop" || value == "all"
}

func newJournal(runID, tableID, domain, targetTable string) (*v1domain.Journal, error) {
	return v1domain.NewJournal(v1domain.Scope{ImportVersion: domainImportVersion, ArchiveRunID: runID,
		AdapterID: v1archive.DefaultAdapterID, TableID: tableID, TargetDomain: domain, TargetTable: targetTable})
}

func importCampaign(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, runID string, actors v1candidate.ActorIDs) (v1domain.CampaignImportResult, error) {
	campaignJournal, err := newJournal(runID, "public/campaigns", "campaign", "cloud_campaigns")
	if err != nil {
		return v1domain.CampaignImportResult{}, err
	}
	stepJournal, err := newJournal(runID, "public/campaign_steps", "campaign", "cloud_campaign_steps")
	if err != nil {
		return v1domain.CampaignImportResult{}, err
	}
	writer, err := campaign.NewHistoricalDefinitionWriter(campaignstore.NewHistoricalDefinitionStore(), campaignJournal)
	if err != nil {
		return v1domain.CampaignImportResult{}, err
	}
	importer, err := v1domain.NewCampaignImporter(archive, uow, writer, campaignJournal, stepJournal, actors)
	if err != nil {
		return v1domain.CampaignImportResult{}, err
	}
	return importer.Import(ctx, runID)
}

func importSurvey(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, runID string, actor int64) (v1domain.SurveyImportResult, error) {
	targets := map[string]string{
		"public/questionnaires": "questionnaires", "public/questionnaire_questions": "questionnaire_questions",
		"public/questionnaire_options": "questionnaire_options", "public/questionnaire_submissions": "questionnaire_submissions",
		"public/questionnaire_submission_answers": "questionnaire_submission_answers",
	}
	journals := make(map[string]*v1domain.Journal, len(targets))
	for table, target := range targets {
		journal, err := newJournal(runID, table, "survey", target)
		if err != nil {
			return v1domain.SurveyImportResult{}, err
		}
		journals[table] = journal
	}
	service := surveyapp.NewImportService(uow, surveystore.NewQuestionnaireRepository())
	importer, err := v1domain.NewSurveyImporter(archive, uow, service, journals, actor)
	if err != nil {
		return v1domain.SurveyImportResult{}, err
	}
	return importer.Import(ctx, runID)
}

func importMedia(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, runID string, actor int64) (v1domain.StaticImportResult, error) {
	journal, err := newJournal(runID, "public/miniprogram_library", "media", "media_miniprograms")
	if err != nil {
		return v1domain.StaticImportResult{}, err
	}
	writer, err := media.NewHistoricalMiniProgramWriter(mediastore.NewHistoricalMiniProgramStore(), journal)
	if err != nil {
		return v1domain.StaticImportResult{}, err
	}
	importer, err := v1domain.NewMiniProgramImporter(archive, uow, writer, journal, actor)
	if err != nil {
		return v1domain.StaticImportResult{}, err
	}
	return importer.Import(ctx, runID)
}

func importRadar(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, runID string, actor int64) (v1domain.StaticImportResult, error) {
	journal, err := newJournal(runID, "public/radar_links", "radar", "radar_links")
	if err != nil {
		return v1domain.StaticImportResult{}, err
	}
	service, err := radarapp.NewService(uow, radarstore.NewPostgresRepository(), rejectingEventAppender{})
	if err != nil {
		return v1domain.StaticImportResult{}, err
	}
	importer, err := v1domain.NewRadarImporter(archive, uow, service, journal, actor)
	if err != nil {
		return v1domain.StaticImportResult{}, err
	}
	return importer.Import(ctx, runID)
}

func importShop(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, runID string) (v1domain.WeChatShopImportResult, error) {
	journal, err := newJournal(runID, "public/wechat_shop_orders", "order", "order_wechat_shop_materials")
	if err != nil {
		return v1domain.WeChatShopImportResult{}, err
	}
	importer, err := v1domain.NewWeChatShopImporter(archive, uow, orderstore.NewWeChatShopMaterialRepository(), journal)
	if err != nil {
		return v1domain.WeChatShopImportResult{}, err
	}
	return importer.Import(ctx, runID)
}

type rejectingEventAppender struct{}

func (rejectingEventAppender) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 0, fmt.Errorf("historical definition attempted to append an event")
}

func parseActorIDs(value string) (v1candidate.ActorIDs, error) {
	result := v1candidate.ActorIDs{}
	for _, pair := range strings.Split(value, ",") {
		parts := strings.Split(pair, "=")
		if len(parts) != 2 || parts[0] == "" || parts[0] != strings.TrimSpace(parts[0]) {
			return nil, fmt.Errorf("invalid campaign actor mapping")
		}
		actorID, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || actorID < 1 {
			return nil, fmt.Errorf("invalid campaign actor mapping")
		}
		if _, exists := result[parts[0]]; exists {
			return nil, fmt.Errorf("duplicate campaign actor mapping")
		}
		result[parts[0]] = actorID
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("campaign actor mapping is required")
	}
	return result, nil
}
