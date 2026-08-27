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
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignstore "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1candidate"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1domain"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const campaignImportVersion = "v1-domain-a1"

func main() {
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	if err := run(os.Args[1:], environment); err != nil {
		fmt.Fprintln(os.Stderr, "aicrm-v1-domain-import:", err)
		os.Exit(2)
	}
}

func run(args []string, environment appconfig.V1ArchiveRuntime) error {
	flags := flag.NewFlagSet("aicrm-v1-domain-import", flag.ContinueOnError)
	domain := flags.String("domain", "", "campaign")
	archiveRunID := flags.String("archive-run-id", "", "reconciled V1 archive run")
	actorValues := flags.String("campaign-actors", "", "explicit owner_userid=V2_actor_id pairs")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *domain != "campaign" || *archiveRunID == "" || environment.TargetDatabaseURL == "" || len(environment.ArchiveKey) != 32 {
		return fmt.Errorf("domain=campaign, archive-run-id, target database and 32-byte archive key are required")
	}
	actors, err := parseActorIDs(*actorValues)
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, environment.TargetDatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		return err
	}
	defer archive.Close()
	campaignJournal, err := v1domain.NewJournal(v1domain.Scope{
		ImportVersion: campaignImportVersion, ArchiveRunID: *archiveRunID,
		AdapterID: v1archive.DefaultAdapterID, TableID: "public/campaigns",
		TargetDomain: "campaign", TargetTable: "cloud_campaigns",
	})
	if err != nil {
		return err
	}
	stepJournal, err := v1domain.NewJournal(v1domain.Scope{
		ImportVersion: campaignImportVersion, ArchiveRunID: *archiveRunID,
		AdapterID: v1archive.DefaultAdapterID, TableID: "public/campaign_steps",
		TargetDomain: "campaign", TargetTable: "cloud_campaign_steps",
	})
	if err != nil {
		return err
	}
	writer, err := campaign.NewHistoricalDefinitionWriter(campaignstore.NewHistoricalDefinitionStore(), campaignJournal)
	if err != nil {
		return err
	}
	importer, err := v1domain.NewCampaignImporter(archive, platformstore.NewUnitOfWork(pool), writer, campaignJournal, stepJournal, actors)
	if err != nil {
		return err
	}
	result, err := importer.Import(ctx, *archiveRunID)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
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
