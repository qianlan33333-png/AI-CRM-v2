package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	adminopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/port"
	adminopsdb "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var ErrNotFound = errors.New("admin ops record not found")

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

func (repository *Repository) CreateCredential(ctx context.Context, credential adminopsport.Credential) (adminopsport.Credential, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return adminopsport.Credential{}, err
	}
	row, err := queries.CreateCredential(ctx, adminopsdb.CreateCredentialParams{CredentialKind: string(credential.Kind), ClientID: credential.ClientID, DisplayName: credential.DisplayName, State: credential.State, SecretRef: credential.SecretRef, SecretMask: credential.SecretMask, Metadata: credential.Metadata, CreatedBy: credential.CreatedBy, CreatedAt: timestamp(credential.CreatedAt)})
	return credentialFromRow(row), mapError(err)
}

func (repository *Repository) GetCredential(ctx context.Context, kind adminopsport.CredentialKind, clientID string) (adminopsport.Credential, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return adminopsport.Credential{}, err
	}
	row, err := queries.GetCredential(ctx, adminopsdb.GetCredentialParams{CredentialKind: string(kind), ClientID: clientID})
	return credentialFromRow(row), mapError(err)
}

func (repository *Repository) ListCredentials(ctx context.Context) ([]adminopsport.Credential, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListCredentials(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	result := make([]adminopsport.Credential, len(rows))
	for index, row := range rows {
		result[index] = credentialFromRow(row)
	}
	return result, nil
}

func (repository *Repository) UpdateCredential(ctx context.Context, credential adminopsport.Credential) (adminopsport.Credential, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return adminopsport.Credential{}, err
	}
	row, err := queries.UpdateCredential(ctx, adminopsdb.UpdateCredentialParams{CredentialKind: string(credential.Kind), ClientID: credential.ClientID, DisplayName: credential.DisplayName, State: credential.State, SecretRef: credential.SecretRef, SecretMask: credential.SecretMask, Metadata: credential.Metadata, UpdatedAt: timestamp(credential.UpdatedAt)})
	return credentialFromRow(row), mapError(err)
}

func (repository *Repository) UpsertCategory(ctx context.Context, category adminopsport.Category) (adminopsport.Category, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return adminopsport.Category{}, err
	}
	row, err := queries.UpsertCategory(ctx, adminopsdb.UpsertCategoryParams{CategoryKey: category.Key, Enabled: category.Enabled, Settings: category.Settings, UpdatedBy: category.UpdatedBy, UpdatedAt: timestamp(category.UpdatedAt)})
	return categoryFromRow(row), mapError(err)
}

func (repository *Repository) GetCategory(ctx context.Context, key string) (adminopsport.Category, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return adminopsport.Category{}, err
	}
	row, err := queries.GetCategory(ctx, key)
	return categoryFromRow(row), mapError(err)
}

func (repository *Repository) ListCategories(ctx context.Context) ([]adminopsport.Category, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListCategories(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	result := make([]adminopsport.Category, len(rows))
	for index, row := range rows {
		result[index] = categoryFromRow(row)
	}
	return result, nil
}

func (repository *Repository) CreateRelease(ctx context.Context, release adminopsport.Release) (adminopsport.Release, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return adminopsport.Release{}, err
	}
	row, err := queries.CreateRelease(ctx, adminopsdb.CreateReleaseParams{State: release.State, Changes: release.Changes, Checksum: release.Checksum, BasedOnReleaseID: nullableInt(release.BasedOnReleaseID), RollbackOfReleaseID: nullableInt(release.RollbackOfReleaseID), CreatedBy: release.CreatedBy, CreatedAt: timestamp(release.CreatedAt)})
	return releaseFromRow(row), mapError(err)
}

func (repository *Repository) GetRelease(ctx context.Context, id int64) (adminopsport.Release, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return adminopsport.Release{}, err
	}
	row, err := queries.GetRelease(ctx, id)
	return releaseFromRow(row), mapError(err)
}

func (repository *Repository) ListReleases(ctx context.Context, limit int32) ([]adminopsport.Release, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListReleases(ctx, limit)
	if err != nil {
		return nil, mapError(err)
	}
	result := make([]adminopsport.Release, len(rows))
	for index, row := range rows {
		result[index] = releaseFromRow(row)
	}
	return result, nil
}

func (repository *Repository) ValidateRelease(ctx context.Context, id int64, at time.Time) (adminopsport.Release, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return adminopsport.Release{}, err
	}
	row, err := queries.ValidateRelease(ctx, adminopsdb.ValidateReleaseParams{ID: id, ValidatedAt: timestamp(at)})
	return releaseFromRow(row), mapError(err)
}

func (repository *Repository) PublishRelease(ctx context.Context, id int64, checksum, actor string, at time.Time) (adminopsport.Release, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return adminopsport.Release{}, err
	}
	row, err := queries.PublishRelease(ctx, adminopsdb.PublishReleaseParams{ID: id, Checksum: checksum, PublishedBy: pgtype.Text{String: actor, Valid: true}, PublishedAt: timestamp(at)})
	return releaseFromRow(row), mapError(err)
}

func (repository *Repository) RollbackRelease(ctx context.Context, id int64, actor string, at time.Time) (adminopsport.Release, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return adminopsport.Release{}, err
	}
	row, err := queries.CreateRollbackRelease(ctx, adminopsdb.CreateRollbackReleaseParams{ID: id, CreatedBy: actor, CreatedAt: timestamp(at)})
	return releaseFromRow(row), mapError(err)
}

func (repository *Repository) CreateJob(ctx context.Context, job adminopsport.Job) (adminopsport.Job, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return adminopsport.Job{}, err
	}
	row, err := queries.CreateJob(ctx, adminopsdb.CreateJobParams{JobKey: job.Key, Kind: job.Kind, TargetRef: job.TargetRef, RequestSummary: job.Request, RequestedBy: job.RequestedBy, CreatedAt: timestamp(job.CreatedAt)})
	return jobFromRow(row), mapError(err)
}

func (repository *Repository) GetJob(ctx context.Context, key string) (adminopsport.Job, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return adminopsport.Job{}, err
	}
	row, err := queries.GetJob(ctx, key)
	return jobFromRow(row), mapError(err)
}

func (repository *Repository) ListJobs(ctx context.Context, kind, state string, limit int32) ([]adminopsport.Job, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListJobs(ctx, adminopsdb.ListJobsParams{Column1: kind, Column2: state, Limit: limit})
	if err != nil {
		return nil, mapError(err)
	}
	result := make([]adminopsport.Job, len(rows))
	for index, row := range rows {
		result[index] = jobFromRow(row)
	}
	return result, nil
}

func (repository *Repository) TransitionJob(ctx context.Context, job adminopsport.Job) (adminopsport.Job, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return adminopsport.Job{}, err
	}
	row, err := queries.TransitionJob(ctx, adminopsdb.TransitionJobParams{JobKey: job.Key, State: job.State, FailureCode: nullableText(job.FailureCode), ResultSummary: job.Result, UpdatedAt: timestamp(job.UpdatedAt), CompletedAt: nullableTime(job.CompletedAt), Version: job.Version})
	return jobFromRow(row), mapError(err)
}

func (repository *Repository) GetNotification(ctx context.Context) (adminopsdb.AdminOpsNotificationSetting, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return adminopsdb.AdminOpsNotificationSetting{}, err
	}
	row, err := queries.GetNotificationSetting(ctx, "feishu")
	return row, mapError(err)
}

func (repository *Repository) UpsertNotification(ctx context.Context, enabled bool, secretRef, secretMask, state, actor string, at time.Time) (adminopsdb.AdminOpsNotificationSetting, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return adminopsdb.AdminOpsNotificationSetting{}, err
	}
	row, err := queries.UpsertNotificationSetting(ctx, adminopsdb.UpsertNotificationSettingParams{Channel: "feishu", Enabled: enabled, SecretRef: secretRef, SecretMask: secretMask, ValidationState: state, UpdatedBy: actor, UpdatedAt: timestamp(at)})
	return row, mapError(err)
}

type Receipt struct {
	ID                               int64
	Action, Actor, State             string
	KeyDigest, PayloadDigest, Result []byte
}

func (repository *Repository) ReserveReceipt(ctx context.Context, action, actor string, key, payload []byte, at time.Time) (Receipt, bool, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return Receipt{}, false, err
	}
	row, err := queries.ReserveActionReceipt(ctx, adminopsdb.ReserveActionReceiptParams{Action: action, ActorScope: actor, KeyDigest: key, PayloadDigest: payload, CreatedAt: timestamp(at)})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, getErr := queries.GetActionReceipt(ctx, adminopsdb.GetActionReceiptParams{Action: action, ActorScope: actor, KeyDigest: key})
		if getErr != nil {
			return Receipt{}, false, mapError(getErr)
		}
		return receiptFromRow(existing), false, nil
	}
	return receiptFromRow(row), err == nil, mapError(err)
}

func (repository *Repository) CompleteReceipt(ctx context.Context, id int64, result []byte, at time.Time) (Receipt, error) {
	queries, err := queries(ctx, repository)
	if err != nil {
		return Receipt{}, err
	}
	row, err := queries.CompleteActionReceipt(ctx, adminopsdb.CompleteActionReceiptParams{ID: id, ResultSnapshot: result, CompletedAt: timestamp(at)})
	return receiptFromRow(row), mapError(err)
}

func queries(ctx context.Context, repository *Repository) (*adminopsdb.Queries, error) {
	if repository == nil {
		return nil, errors.New("admin ops repository is required")
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return adminopsdb.New(tx), nil
}

func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func nullableTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamp(*value)
}

func nullableInt(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func nullableText(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func timePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	item := value.Time.UTC()
	return &item
}

func intPtr(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	item := value.Int64
	return &item
}

func credentialFromRow(row adminopsdb.AdminOpsCredential) adminopsport.Credential {
	return adminopsport.Credential{ID: row.ID, Kind: adminopsport.CredentialKind(row.CredentialKind), ClientID: row.ClientID, DisplayName: row.DisplayName, State: row.State, SecretRef: row.SecretRef, SecretMask: row.SecretMask, Metadata: row.Metadata, Version: row.Version, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}
}

func categoryFromRow(row adminopsdb.AdminOpsConfigCategory) adminopsport.Category {
	return adminopsport.Category{Key: row.CategoryKey, Enabled: row.Enabled, Settings: row.Settings, Version: row.Version, UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt.Time.UTC()}
}

func releaseFromRow(row adminopsdb.AdminOpsConfigRelease) adminopsport.Release {
	return adminopsport.Release{ID: row.ID, State: row.State, Changes: row.Changes, Checksum: row.Checksum, BasedOnReleaseID: intPtr(row.BasedOnReleaseID), RollbackOfReleaseID: intPtr(row.RollbackOfReleaseID), CreatedBy: row.CreatedBy, PublishedBy: row.PublishedBy.String, CreatedAt: row.CreatedAt.Time.UTC(), ValidatedAt: timePtr(row.ValidatedAt), PublishedAt: timePtr(row.PublishedAt)}
}

func jobFromRow(row adminopsdb.AdminOpsJob) adminopsport.Job {
	return adminopsport.Job{ID: row.ID, Key: row.JobKey, Kind: row.Kind, State: row.State, TargetRef: row.TargetRef, Request: row.RequestSummary, Result: row.ResultSummary, Version: row.Version, RequestedBy: row.RequestedBy, FailureCode: row.FailureCode.String, CreatedAt: row.CreatedAt.Time.UTC(), StartedAt: timePtr(row.StartedAt), CompletedAt: timePtr(row.CompletedAt), UpdatedAt: row.UpdatedAt.Time.UTC()}
}

func receiptFromRow(row adminopsdb.AdminOpsActionReceipt) Receipt {
	return Receipt{ID: row.ID, Action: row.Action, Actor: row.ActorScope, State: row.State, KeyDigest: row.KeyDigest, PayloadDigest: row.PayloadDigest, Result: row.ResultSnapshot}
}
