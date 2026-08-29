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
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1membergridhistory"
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
const staticImportVersion = "v1-static-a1"
const financeImportVersion = "v1-finance-a1"
const servicePeriodImportVersion = "v1-service-period-a1"
const couponImportVersion = "v1-coupon-a1"

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
	domain := flags.String("domain", "", "campaign|survey|media|radar|shop|all (first package)|static (Contact/Product/media blobs)|finance|coupon (read-only history)|service-period (read-only history)|channel (inactive definitions and history)|groupops (read-only history)|audience-history (non-executable history)|message-history (masked read-only history)|contact-history (read-only snapshots)|customer-timeline-history (immutable observations)|member-grid-history (read-only)|campaign-history (read-only snapshots)|automation-history (non-executable configuration)|profile-catalog-history (inert templates and signup rules)|hxc-history (immutable observations)|hxc-runtime-history (inert sender/send observations)|hxc-chat-job-history (inert dialogue job observations)|hxc-member-usage-history (inert generation observations)|contact-reference-history (inert customer binding/directory references)|cycle-observation-history (read-only cycle metrics/references)|static-tail-history (inert media/product/cycle facts)|customer-state-history (immutable status observations)|marketing-state-history (immutable marketing observations)|legacy-marketing-history (read-only snapshots)|survey-unresolved-history (read-only answers)|broadcast-job-history (inert job observations)|outbound-task-history (inert task observations)|external-identity-gap (sealed archive gap identities)|wecom-contact-history (read-only source observations)|radar-click-history|marketing-config-history")
	archiveRunID := flags.String("archive-run-id", "", "reconciled V1 archive run")
	actorValues := flags.String("campaign-actors", "", "explicit owner_userid=V2_actor_id pairs")
	migrationActor := flags.Int64("migration-actor", 0, "explicit V2 actor for local historical definitions")
	dm01RunID := flags.Int64("dm01-run-id", 0, "verified DM01 full-import run for historical references")
	referenceCorpID := flags.String("reference-corp-id", "", "explicit WeCom enterprise for contact-reference-history attribution")
	usageRecoveryFile := flags.String("usage-recovery-file", "", "frozen has_token_usage recovery JSONL for member-grid-history import")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if (*mode != "import" && *mode != "reconcile") || !validDomain(*domain) || *archiveRunID == "" || environment.TargetDatabaseURL == "" {
		return fmt.Errorf("valid mode, domain, archive-run-id and target database are required")
	}
	if *mode == "import" && len(environment.ArchiveKey) != 32 {
		return fmt.Errorf("32-byte archive key is required for import")
	}
	if *mode == "reconcile" && *domain != "hxc-chat-job-history" && *domain != "hxc-member-usage-history" && *domain != "contact-reference-history" && *domain != "cycle-observation-history" && *domain != "hxc-runtime-history" && *domain != "invalid-source-history" && *domain != deferredIdentityHistoryDomain && *domain != campaignDefinitionHistoryDomain && *domain != "outbound-task-history" && *domain != customerTimelineHistoryDomain && *domain != "all" && *domain != "static" && *domain != "finance" && *domain != "channel" && *domain != "service-period" && *domain != "coupon" && *domain != "groupops" && *domain != "audience-history" && *domain != "message-history" && *domain != "contact-history" && *domain != "member-grid-history" && *domain != "campaign-history" && *domain != "automation-history" && *domain != "profile-catalog-history" && *domain != "hxc-history" && *domain != "static-tail-history" && *domain != "customer-state-history" && *domain != "marketing-state-history" && *domain != "survey-unresolved-history" && *domain != "legacy-marketing-history" && *domain != "broadcast-job-history" && *domain != "external-identity-gap" && *domain != weComContactHistoryDomain && *domain != v1domain.RadarClickHistoryDomain && *domain != v1domain.MarketingConfigHistoryDomain {
		return fmt.Errorf("reconcile requires domain=cycle-observation-history, all, static, finance, channel, service-period, coupon, groupops, audience-history contact-history, message-history member-grid-history campaign-history or automation-history or profile-catalog-history or hxc-history or static-tail-history or customer-state-history or marketing-state-history or survey-unresolved-history or legacy-marketing-history or broadcast-job-history or external-identity-gap or wecom-contact-history or radar-click-history or marketing-config-history")
	}
	var actors v1candidate.ActorIDs
	if *domain == "hxc-chat-job-history" && (environment.SourceDatabaseURL != "" || len(environment.SourceHMACKey) < 32 || len(environment.ArchiveKey) != 32) {
		return fmt.Errorf("hxc-chat-job-history requires local-only archive keys")
	}
	if *domain == "hxc-member-usage-history" && (environment.SourceDatabaseURL != "" || len(environment.SourceHMACKey) < 32 || len(environment.ArchiveKey) != 32) {
		return fmt.Errorf("hxc-member-usage-history requires local-only archive keys")
	}
	if *domain == campaignDefinitionHistoryDomain && len(environment.SourceHMACKey) < 32 {
		return fmt.Errorf("campaign-definition-history requires the frozen archive source HMAC key")
	}
	var err error
	if *mode == "import" && (*domain == "campaign" || *domain == "all") {
		actors, err = parseActorIDs(*actorValues)
		if err != nil {
			return err
		}
	}
	if *mode == "import" && (*domain == "survey" || *domain == "media" || *domain == "radar" || *domain == "all" || *domain == "static" || *domain == "channel" || *domain == "coupon" || *domain == "groupops" || *domain == "audience-history") && *migrationActor < 1 {
		return fmt.Errorf("migration-actor is required")
	}
	var dm01HMACKey []byte
	if *mode == "import" && (*domain == "static" || *domain == "finance" || *domain == "channel" || *domain == "coupon" || *domain == "service-period" || *domain == "groupops" || *domain == "audience-history" || *domain == "message-history" || *domain == "contact-history" || *domain == "member-grid-history" || *domain == "campaign-history" || *domain == "hxc-history" || *domain == "survey-unresolved-history" || *domain == v1domain.RadarClickHistoryDomain) {
		dm01HMACKey = []byte(appconfig.LoadDM01RuntimeEnvironment().SourceHMACKey)
		if *dm01RunID < 1 || len(dm01HMACKey) < 32 {
			return fmt.Errorf("%s import requires dm01-run-id and the frozen DM01 source HMAC key", *domain)
		}
	}
	var usageRecovery []v1membergridhistory.UsageSnapshotRecoveryEntry
	if *mode == "import" && *domain == v1domain.RadarClickHistoryDomain && len(environment.SourceHMACKey) < 32 {
		return fmt.Errorf("radar-click-history requires the frozen archive source HMAC key")
	}
	if *mode == "import" && *domain == "member-grid-history" {
		if len(environment.SourceHMACKey) < 32 {
			return fmt.Errorf("member-grid-history requires the frozen archive source HMAC key")
		}
		usageRecovery, err = loadMemberGridHistoryRecovery(*usageRecoveryFile, *archiveRunID)
		if err != nil {
			return err
		}
	}
	ctx := context.Background()
	if *domain == "external-identity-gap" || *domain == deferredIdentityHistoryDomain || *domain == "contact-reference-history" {
		dm01 := appconfig.LoadDM01RuntimeEnvironment()
		dm01HMACKey = []byte(dm01.SourceHMACKey)
		if environment.SourceDatabaseURL != "" || dm01.SourceDatabaseURL != "" || *dm01RunID < 1 || len(dm01HMACKey) < 32 || len(environment.SourceHMACKey) < 32 || len(environment.ArchiveKey) != 32 {
			return fmt.Errorf("%s requires local-only archive/DM01 keys and a completed DM01 run", *domain)
		}
	}
	if *domain == "legacy-marketing-history" && *mode == "import" && len(environment.SourceHMACKey) < 32 {
		return fmt.Errorf("legacy-marketing-history requires the frozen archive source HMAC key")
	}
	if *domain == "cycle-observation-history" && (environment.SourceDatabaseURL != "" || len(environment.SourceHMACKey) < 32 || len(environment.ArchiveKey) != 32) {
		return fmt.Errorf("cycle-observation-history requires local-only archive keys")
	}
	if *domain == "contact-reference-history" && (*referenceCorpID == "" || strings.TrimSpace(*referenceCorpID) != *referenceCorpID) {
		return fmt.Errorf("contact-reference-history requires reference-corp-id")
	}
	if *domain == customerTimelineHistoryDomain {
		dm01 := appconfig.LoadDM01RuntimeEnvironment()
		dm01HMACKey = []byte(dm01.SourceHMACKey)
		if environment.SourceDatabaseURL != "" || dm01.SourceDatabaseURL != "" || *dm01RunID < 1 || len(environment.SourceHMACKey) < 32 || len(environment.ArchiveKey) != 32 || len(dm01HMACKey) < 32 {
			return fmt.Errorf("customer-timeline-history requires local-only archive/DM01 keys and a completed DM01 run")
		}
	}
	pool, err := pgxpool.New(ctx, environment.TargetDatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if *domain == "hxc-member-usage-history" {
		archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
		if err != nil {
			return err
		}
		defer archive.Close()
		value, err := v1domain.RunHXCMemberUsageHistory(ctx, pool, archive, *archiveRunID, []byte(environment.SourceHMACKey), *mode == "reconcile")
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"hxc_member_usage_history": value})
	}
	if *domain == "hxc-chat-job-history" {
		archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
		if err != nil {
			return err
		}
		defer archive.Close()
		value, err := v1domain.RunHXCChatJobHistory(ctx, pool, archive, *archiveRunID, []byte(environment.SourceHMACKey), *mode == "reconcile")
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"hxc_chat_job_history": value})
	}
	if *domain == "cycle-observation-history" {
		archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
		if err != nil {
			return err
		}
		defer archive.Close()
		value, err := v1domain.RunCycleObservationHistory(ctx, pool, archive, *archiveRunID, []byte(environment.SourceHMACKey), *mode == "reconcile")
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"cycle_observation_history": value})
	}
	if *domain == "contact-reference-history" {
		archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
		if err != nil {
			return err
		}
		defer archive.Close()
		resolver, err := newContactReferenceResolver(ctx, platformstore.NewUnitOfWork(pool), archive, *archiveRunID, *dm01RunID, []byte(environment.SourceHMACKey), dm01HMACKey, *referenceCorpID)
		if err != nil {
			return err
		}
		value, err := v1domain.RunContactReferenceHistory(ctx, pool, archive, *archiveRunID, []byte(environment.SourceHMACKey), resolver, *mode == "reconcile")
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"contact_reference_history": value})
	}
	if *domain == customerTimelineHistoryDomain {
		archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
		if err != nil {
			return err
		}
		defer archive.Close()
		value, err := importCustomerTimelineHistory(ctx, archive, platformstore.NewUnitOfWork(pool), *archiveRunID, *dm01RunID, dm01HMACKey, []byte(environment.SourceHMACKey), *mode == "reconcile")
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"customer_timeline_history": value})
	}
	if *domain == "hxc-runtime-history" {
		if environment.SourceDatabaseURL != "" || len(environment.SourceHMACKey) < 32 || len(environment.ArchiveKey) != 32 {
			return fmt.Errorf("hxc-runtime-history requires local-only archive keys")
		}
		archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
		if err != nil {
			return err
		}
		defer archive.Close()
		value, err := v1domain.RunHXCRuntimeHistory(ctx, pool, archive, *archiveRunID, []byte(environment.SourceHMACKey), *mode == "reconcile")
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"hxc_runtime_history": value})
	}
	if *domain == "invalid-source-history" {
		if environment.SourceDatabaseURL != "" || len(environment.SourceHMACKey) < 32 || len(environment.ArchiveKey) != 32 {
			return fmt.Errorf("invalid-source-history requires local-only archive keys")
		}
		archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
		if err != nil {
			return err
		}
		defer archive.Close()
		value, err := v1domain.RunInvalidSourceHistory(ctx, pool, archive, *archiveRunID, []byte(environment.SourceHMACKey), *mode == "reconcile")
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"invalid_source_history": value})
	}
	if *domain == deferredIdentityHistoryDomain {
		archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
		if err != nil {
			return err
		}
		defer archive.Close()
		return runDeferredIdentityHistory(ctx, pool, archive, *mode, *archiveRunID, *dm01RunID, []byte(environment.SourceHMACKey), dm01HMACKey)
	}
	if *domain == "external-identity-gap" {
		archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
		if err != nil {
			return err
		}
		defer archive.Close()
		importer, err := newExternalIdentityGapImporter(archive, platformstore.NewUnitOfWork(pool), *archiveRunID)
		if err != nil {
			return err
		}
		options := v1domain.ExternalIdentityGapImportOptions{ArchiveRunID: *archiveRunID, DM01RunID: *dm01RunID, SourceHMACKey: []byte(environment.SourceHMACKey), DM01SourceHMACKey: dm01HMACKey, TargetHMACKey: []byte(environment.SourceHMACKey), KeyVersion: 1}
		if *mode == "reconcile" {
			value, err := v1domain.ReconcileExternalIdentityGap(ctx, pool, importer, options)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"external_identity_gap_reconciliation": value})
		}
		value, err := importer.Import(ctx, options)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"external_identity_gap": value})
	}
	if *mode == "reconcile" {
		if *domain == "outbound-task-history" {
			value, err := v1domain.ReconcileOutboundTaskHistory(ctx, pool, outboundTaskHistoryImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"outbound_task_history_reconciliation": value})
		}
		if *domain == "survey-unresolved-history" {
			value, err := v1domain.ReconcileSurveyUnresolvedHistory(ctx, pool, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"survey_unresolved_history_reconciliation": value})
		}
		if *domain == "legacy-marketing-history" {
			result, err := v1domain.ReconcileLegacyMarketingHistory(ctx, pool, legacyMarketingHistoryImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"legacy_marketing_history_reconciliation": result})
		}
		if *domain == "broadcast-job-history" {
			value, err := v1domain.ReconcileBroadcastJobHistory(ctx, pool, broadcastJobHistoryImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"broadcast_job_history_reconciliation": value})
		}
		if *domain == "profile-catalog-history" {
			result, err := v1domain.ReconcileProfileCatalogHistory(ctx, pool, profileCatalogHistoryImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"profile_catalog_history_reconciliation": result})
		}
		if *domain == v1domain.RadarClickHistoryDomain {
			result, err := v1domain.ReconcileRadarClickHistory(ctx, pool, v1domain.RadarClickHistoryImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"radar_click_history_reconciliation": result})
		}
		if *domain == v1domain.MarketingConfigHistoryDomain {
			result, err := v1domain.ReconcileMarketingConfigHistory(ctx, pool, v1domain.MarketingConfigHistoryImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"marketing_config_history_reconciliation": result})
		}
		if *domain == "automation-history" {
			result, err := v1domain.ReconcileAutomationHistory(ctx, pool, automationHistoryImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"automation_history_reconciliation": result})
		}
		if *domain == weComContactHistoryDomain {
			result, err := v1domain.ReconcileWeComContactHistory(ctx, pool, weComContactHistoryImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"wecom_contact_history_reconciliation": result})
		}
		if *domain == "message-history" {
			result, err := v1domain.ReconcileMessageHistory(ctx, pool, messageHistoryImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"message_history_reconciliation": result})
		}
		if *domain == "audience-history" {
			result, err := v1domain.ReconcileAudienceHistory(ctx, pool, v1domain.AudienceHistoryImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"audience_history_reconciliation": result})
		}
		if *domain == "member-grid-history" {
			result, err := v1domain.ReconcileMemberGridHistory(ctx, pool, memberGridHistoryImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"member_grid_history_reconciliation": result})
		}
		if *domain == "marketing-state-history" {
			result, err := v1domain.ReconcileMarketingStateHistory(ctx, pool, marketingStateHistoryImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"marketing_state_history_reconciliation": result})
		}
		if *domain == "customer-state-history" {
			result, err := v1domain.ReconcileCustomerStateHistory(ctx, pool, customerStateHistoryImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"customer_state_history_reconciliation": result})
		}
		if *domain == "static-tail-history" {
			result, err := v1domain.ReconcileStaticTailHistory(ctx, pool, staticTailHistoryImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"static_tail_history_reconciliation": result})
		}
		if *domain == "hxc-history" {
			result, err := v1domain.ReconcileHXCHistory(ctx, pool, hxcHistoryImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"hxc_history_reconciliation": result})
		}
		if *domain == "contact-history" {
			result, err := v1domain.ReconcileContactHistory(ctx, pool, contactHistoryImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"contact_history_reconciliation": result})
		}
		if *domain == "campaign-history" {
			result, err := v1domain.ReconcileCampaignHistory(ctx, pool, campaignHistoryImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"campaign_history_reconciliation": result})
		}
		if *domain == campaignDefinitionHistoryDomain {
			result, err := v1domain.ReconcileCampaignDefinitionHistory(ctx, pool, campaignDefinitionHistoryImportVersion, *archiveRunID, []byte(environment.SourceHMACKey))
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"campaign_definition_history_reconciliation": result})
		}
		if *domain == "channel" {
			result, err := v1domain.ReconcileChannel(ctx, pool, channelImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"channel_reconciliation": result})
		}
		if *domain == "service-period" {
			result, err := v1domain.ReconcileServicePeriod(ctx, pool, servicePeriodImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"service_period_reconciliation": result})
		}
		if *domain == "coupon" {
			result, err := v1domain.ReconcileCoupon(ctx, pool, couponImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"coupon_reconciliation": result})
		}
		if *domain == "groupops" {
			result, err := v1domain.ReconcileGroupOps(ctx, pool, groupOpsHistoryImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"groupops_reconciliation": result})
		}
		if *domain == "static" {
			result, err := v1domain.ReconcileStatic(ctx, pool, staticImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"static_reconciliation": result})
		}
		if *domain == "finance" {
			result, err := v1domain.ReconcileFinance(ctx, pool, financeImportVersion, *archiveRunID)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"finance_reconciliation": result})
		}
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
	if *domain == "static" {
		result, err := importStatic(ctx, archive, uow, *archiveRunID, *migrationActor, *dm01RunID, dm01HMACKey)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	result := map[string]any{}
	if *domain == "outbound-task-history" {
		if environment.SourceDatabaseURL != "" || len(environment.SourceHMACKey) < 32 {
			return fmt.Errorf("outbound-task-history requires local-only archive keys")
		}
		value, err := importOutboundTaskHistory(ctx, archive, uow, *archiveRunID, []byte(environment.SourceHMACKey))
		if err != nil {
			return err
		}
		result["outbound_task_history"] = value
	}
	if *domain == "survey-unresolved-history" {
		value, err := importSurveyUnresolvedHistory(ctx, archive, uow, *archiveRunID, *dm01RunID, dm01HMACKey, []byte(environment.SourceHMACKey))
		if err != nil {
			return err
		}
		result["survey_unresolved_history"] = value
	}
	if *domain == "legacy-marketing-history" {
		value, err := importLegacyMarketingHistory(ctx, archive, uow, *archiveRunID, []byte(environment.SourceHMACKey))
		if err != nil {
			return err
		}
		result["legacy_marketing_history"] = value
	}
	if *domain == "broadcast-job-history" {
		value, err := importBroadcastJobHistory(ctx, archive, uow, *archiveRunID)
		if err != nil {
			return err
		}
		result["broadcast_job_history"] = value
	}
	if *domain == "profile-catalog-history" {
		value, err := importProfileCatalogHistory(ctx, archive, uow, *archiveRunID)
		if err != nil {
			return err
		}
		result["profile_catalog_history"] = value
	}
	if *domain == v1domain.RadarClickHistoryDomain {
		value, err := importRadarClickHistory(ctx, archive, uow, *archiveRunID, *dm01RunID, dm01HMACKey, []byte(environment.SourceHMACKey))
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"radar_click_history": value})
	}
	if *domain == v1domain.MarketingConfigHistoryDomain {
		value, err := importMarketingConfigHistory(ctx, archive, uow, *archiveRunID)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"marketing_config_history": value})
	}
	if *domain == "automation-history" {
		value, err := importAutomationHistory(ctx, archive, uow, *archiveRunID)
		if err != nil {
			return err
		}
		result["automation_history"] = value
	}
	if *domain == weComContactHistoryDomain {
		value, err := importWeComContactHistory(ctx, archive, uow, *archiveRunID)
		if err != nil {
			return err
		}
		result["wecom_contact_history"] = value
	}
	if *domain == "member-grid-history" {
		value, err := importMemberGridHistory(ctx, pool, archive, uow, *archiveRunID, *dm01RunID, dm01HMACKey, []byte(environment.SourceHMACKey), usageRecovery)
		if err != nil {
			return err
		}
		result["member_grid_history"] = value
	}
	if *domain == "marketing-state-history" {
		value, err := importMarketingStateHistory(ctx, archive, uow, *archiveRunID)
		if err != nil {
			return err
		}
		result["marketing_state_history"] = value
	}
	if *domain == "customer-state-history" {
		value, err := importCustomerStateHistory(ctx, archive, uow, *archiveRunID)
		if err != nil {
			return err
		}
		result["customer_state_history"] = value
	}
	if *domain == "static-tail-history" {
		value, err := importStaticTailHistory(ctx, archive, uow, *archiveRunID)
		if err != nil {
			return err
		}
		result["static_tail_history"] = value
	}
	if *domain == "hxc-history" {
		value, err := importHXCHistory(ctx, archive, uow, *archiveRunID, *dm01RunID, dm01HMACKey)
		if err != nil {
			return err
		}
		result["hxc_history"] = value
	}
	if *domain == "contact-history" {
		value, err := importContactHistory(ctx, archive, uow, *archiveRunID, *dm01RunID, dm01HMACKey)
		if err != nil {
			return err
		}
		result["contact_history"] = value
	}
	if *domain == "message-history" {
		value, err := importMessageHistory(ctx, archive, uow, *archiveRunID, *dm01RunID, dm01HMACKey)
		if err != nil {
			return err
		}
		result["message_history"] = value
	}
	if *domain == "campaign-history" {
		value, err := importCampaignHistory(ctx, archive, uow, *archiveRunID, *dm01RunID, dm01HMACKey)
		if err != nil {
			return err
		}
		result["campaign_history"] = value
	}
	if *domain == campaignDefinitionHistoryDomain {
		value, err := importCampaignDefinitionHistory(ctx, pool, archive, uow, *archiveRunID, []byte(environment.SourceHMACKey))
		if err != nil {
			return err
		}
		result["campaign_definition_history"] = value
	}
	if *domain == "service-period" {
		value, err := importServicePeriod(ctx, archive, uow, *archiveRunID, *dm01RunID, dm01HMACKey)
		if err != nil {
			return err
		}
		result["service_period"] = value
	}
	if *domain == "coupon" {
		value, err := importCoupon(ctx, archive, uow, *archiveRunID, *migrationActor, *dm01RunID, dm01HMACKey)
		if err != nil {
			return err
		}
		result["coupon"] = value
	}
	if *domain == "groupops" {
		value, err := importGroupOps(ctx, archive, uow, *archiveRunID, *migrationActor, *dm01RunID, dm01HMACKey)
		if err != nil {
			return err
		}
		result["groupops"] = value
	}
	if *domain == "audience-history" {
		value, err := importAudienceHistory(ctx, archive, uow, *archiveRunID, *migrationActor, *dm01RunID, dm01HMACKey)
		if err != nil {
			return err
		}
		result["audience_history"] = value
	}
	if *domain == "finance" {
		value, err := importFinance(ctx, archive, uow, *archiveRunID, *dm01RunID, dm01HMACKey)
		if err != nil {
			return err
		}
		result["finance"] = value
	}
	if *domain == "channel" {
		value, err := importChannel(ctx, archive, uow, *archiveRunID, *migrationActor, *dm01RunID, dm01HMACKey)
		if err != nil {
			return err
		}
		result["channel"] = value
	}
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
	if value == customerTimelineHistoryDomain || value == "contact-reference-history" || value == "cycle-observation-history" || value == deferredIdentityHistoryDomain || value == campaignDefinitionHistoryDomain {
		return true
	}
	return value == "hxc-chat-job-history" || value == "hxc-member-usage-history" || value == "hxc-runtime-history" || value == "invalid-source-history" || value == "outbound-task-history" || value == "campaign" || value == "survey" || value == "media" || value == "radar" || value == "shop" || value == "all" || value == "static" || value == "finance" || value == "channel" || value == "service-period" || value == "coupon" || value == "groupops" || value == "audience-history" || value == "member-grid-history" || value == "message-history" || value == "contact-history" || value == "campaign-history" || value == "automation-history" || value == "profile-catalog-history" || value == "hxc-history" || value == "static-tail-history" || value == "customer-state-history" || value == "marketing-state-history" || value == "survey-unresolved-history" || value == "legacy-marketing-history" || value == "broadcast-job-history" || value == "external-identity-gap" || value == weComContactHistoryDomain || value == v1domain.RadarClickHistoryDomain || value == v1domain.MarketingConfigHistoryDomain
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
