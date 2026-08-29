package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
	wecomdb "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store/generated"
)

// CustomerAcquisitionLinkReceiptRepository persists only the customer-
// acquisition link command/receipt boundary. It neither owns staff projection
// nor creates a generic Provider workflow.
type CustomerAcquisitionLinkReceiptRepository struct {
	pool *pgxpool.Pool
	uow  *platformstore.UnitOfWork
}

var _ wecomapp.CustomerAcquisitionLinkReceiptStore = (*CustomerAcquisitionLinkReceiptRepository)(nil)

func NewCustomerAcquisitionLinkReceiptRepository(pool *pgxpool.Pool) *CustomerAcquisitionLinkReceiptRepository {
	return &CustomerAcquisitionLinkReceiptRepository{pool: pool, uow: platformstore.NewUnitOfWork(pool)}
}

func (repository *CustomerAcquisitionLinkReceiptRepository) ReserveCustomerAcquisitionLink(ctx context.Context, operation wecomapp.CustomerAcquisitionLinkOperation, command wecomapp.CustomerAcquisitionLinkCommand, requestDigest [32]byte) (wecomapp.CustomerAcquisitionLinkReceipt, error) {
	if repository == nil || repository.pool == nil || repository.uow == nil || ctx == nil || command.Actor < 1 {
		return wecomapp.CustomerAcquisitionLinkReceipt{}, wecomapp.ErrCustomerAcquisitionLinkUnavailable
	}
	input, err := json.Marshal(customerAcquisitionLinkStoredInput{LinkName: command.Input.LinkName, UserIDs: command.Input.UserIDs, DepartmentIDs: command.Input.DepartmentIDs, SkipVerify: command.Input.SkipVerify})
	if err != nil {
		return wecomapp.CustomerAcquisitionLinkReceipt{}, wecomapp.ErrCustomerAcquisitionLinkUnavailable
	}
	keyDigest := sha256.Sum256([]byte(command.IdempotencyKey))
	var row wecomdb.WecomCustomerAcquisitionLinkReceipt
	err = repository.within(ctx, func(queries *wecomdb.Queries) error {
		row, err = queries.InsertCustomerAcquisitionLinkReceipt(ctx, wecomdb.InsertCustomerAcquisitionLinkReceiptParams{
			ActorID: command.Actor, KeyDigest: keyDigest[:], RequestDigest: requestDigest[:], Operation: string(operation), LinkID: command.LinkID, CommandInput: input,
		})
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		row, err = queries.GetCustomerAcquisitionLinkReceiptByKey(ctx, wecomdb.GetCustomerAcquisitionLinkReceiptByKeyParams{ActorID: command.Actor, KeyDigest: keyDigest[:]})
		return err
	})
	if err != nil {
		return wecomapp.CustomerAcquisitionLinkReceipt{}, customerAcquisitionLinkReceiptStoreError(err)
	}
	return customerAcquisitionLinkReceipt(row)
}

func (repository *CustomerAcquisitionLinkReceiptRepository) MarkCustomerAcquisitionLinkAttempted(ctx context.Context, id int64) (wecomapp.CustomerAcquisitionLinkReceipt, error) {
	if id < 1 {
		return wecomapp.CustomerAcquisitionLinkReceipt{}, wecomapp.ErrCustomerAcquisitionLinkUnavailable
	}
	var row wecomdb.WecomCustomerAcquisitionLinkReceipt
	err := repository.within(ctx, func(queries *wecomdb.Queries) error {
		var err error
		row, err = queries.MarkCustomerAcquisitionLinkReceiptAttempted(ctx, id)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		row, err = queries.GetCustomerAcquisitionLinkReceipt(ctx, id)
		return err
	})
	if err != nil {
		return wecomapp.CustomerAcquisitionLinkReceipt{}, customerAcquisitionLinkReceiptStoreError(err)
	}
	return customerAcquisitionLinkReceipt(row)
}

func (repository *CustomerAcquisitionLinkReceiptRepository) CompleteCustomerAcquisitionLink(ctx context.Context, completion wecomapp.CustomerAcquisitionLinkCompletion) (wecomapp.CustomerAcquisitionLinkReceipt, error) {
	if completion.ReceiptID < 1 || completion.OutcomeDigest == ([32]byte{}) || !validCustomerAcquisitionLinkReceiptCompletion(completion) {
		return wecomapp.CustomerAcquisitionLinkReceipt{}, wecomapp.ErrCustomerAcquisitionLinkUnavailable
	}
	var providerLink []byte
	if completion.Link != nil {
		var err error
		providerLink, err = json.Marshal(completion.Link)
		if err != nil {
			return wecomapp.CustomerAcquisitionLinkReceipt{}, wecomapp.ErrCustomerAcquisitionLinkUnavailable
		}
	}
	parameters := wecomdb.CompleteCustomerAcquisitionLinkReceiptParams{
		ID: completion.ReceiptID, State: string(completion.State), ProviderLink: providerLink, OutcomeDigest: completion.OutcomeDigest[:],
		BusinessEndpointDispatched: completion.BusinessEndpointDispatched, RealExternalCallExecuted: completion.RealExternalCallExecuted,
	}
	if completion.State == wecomapp.CustomerAcquisitionLinkReconciled {
		parameters.ReconcileActorID = pgtype.Int8{Int64: completion.ReconcileActor, Valid: true}
		parameters.ReconcileKeyDigest = completion.ReconcileKeyDigest[:]
		parameters.EvidenceDigest = completion.EvidenceDigest[:]
		parameters.Resolution = pgtype.Text{String: string(completion.Resolution), Valid: true}
	}
	var row wecomdb.WecomCustomerAcquisitionLinkReceipt
	err := repository.within(ctx, func(queries *wecomdb.Queries) error {
		var err error
		row, err = queries.CompleteCustomerAcquisitionLinkReceipt(ctx, parameters)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		row, err = queries.GetCustomerAcquisitionLinkReceipt(ctx, completion.ReceiptID)
		return err
	})
	if err != nil {
		return wecomapp.CustomerAcquisitionLinkReceipt{}, customerAcquisitionLinkReceiptStoreError(err)
	}
	return customerAcquisitionLinkReceipt(row)
}

func (repository *CustomerAcquisitionLinkReceiptRepository) GetCustomerAcquisitionLinkReceipt(ctx context.Context, id int64) (wecomapp.CustomerAcquisitionLinkReceipt, error) {
	if repository == nil || repository.pool == nil || ctx == nil || id < 1 {
		return wecomapp.CustomerAcquisitionLinkReceipt{}, wecomapp.ErrCustomerAcquisitionLinkUnavailable
	}
	queries := wecomdb.New(repository.pool)
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		queries = wecomdb.New(tx)
	}
	row, err := queries.GetCustomerAcquisitionLinkReceipt(ctx, id)
	if err != nil {
		return wecomapp.CustomerAcquisitionLinkReceipt{}, customerAcquisitionLinkReceiptStoreError(err)
	}
	return customerAcquisitionLinkReceipt(row)
}

func (repository *CustomerAcquisitionLinkReceiptRepository) within(ctx context.Context, callback func(*wecomdb.Queries) error) error {
	if repository == nil || repository.pool == nil || repository.uow == nil || ctx == nil || callback == nil {
		return wecomapp.ErrCustomerAcquisitionLinkUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return callback(wecomdb.New(tx))
	}
	return repository.uow.Within(ctx, func(txCtx context.Context) error {
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		return callback(wecomdb.New(tx))
	})
}

type customerAcquisitionLinkStoredInput struct {
	LinkName      string   `json:"link_name"`
	UserIDs       []string `json:"user_ids"`
	DepartmentIDs []int64  `json:"department_ids"`
	SkipVerify    bool     `json:"skip_verify"`
}

func customerAcquisitionLinkReceipt(row wecomdb.WecomCustomerAcquisitionLinkReceipt) (wecomapp.CustomerAcquisitionLinkReceipt, error) {
	if len(row.RequestDigest) != 32 || len(row.OutcomeDigest) > 0 && len(row.OutcomeDigest) != 32 || len(row.ReconcileKeyDigest) > 0 && len(row.ReconcileKeyDigest) != 32 || len(row.EvidenceDigest) > 0 && len(row.EvidenceDigest) != 32 {
		return wecomapp.CustomerAcquisitionLinkReceipt{}, errors.New("invalid customer acquisition link receipt digest")
	}
	var input customerAcquisitionLinkStoredInput
	if json.Unmarshal(row.CommandInput, &input) != nil {
		return wecomapp.CustomerAcquisitionLinkReceipt{}, errors.New("invalid customer acquisition link receipt command")
	}
	receipt := wecomapp.CustomerAcquisitionLinkReceipt{
		ID: row.ID, Operation: wecomapp.CustomerAcquisitionLinkOperation(row.Operation),
		Command: wecomapp.CustomerAcquisitionLinkCommand{Actor: row.ActorID, LinkID: row.LinkID, Input: wecomport.CustomerAcquisitionLinkInput{LinkName: input.LinkName, UserIDs: input.UserIDs, DepartmentIDs: input.DepartmentIDs, SkipVerify: input.SkipVerify}},
		State:   wecomapp.CustomerAcquisitionLinkReceiptState(row.State), BusinessEndpointDispatched: row.BusinessEndpointDispatched,
		RealExternalCallExecuted: row.RealExternalCallExecuted,
	}
	copy(receipt.RequestDigest[:], row.RequestDigest)
	copy(receipt.OutcomeDigest[:], row.OutcomeDigest)
	copy(receipt.ReconcileKeyDigest[:], row.ReconcileKeyDigest)
	copy(receipt.EvidenceDigest[:], row.EvidenceDigest)
	if row.Resolution.Valid {
		receipt.Resolution = wecomapp.CustomerAcquisitionLinkResolution(row.Resolution.String)
	}
	if len(row.ProviderLink) > 0 {
		var link wecomport.CustomerAcquisitionLink
		if json.Unmarshal(row.ProviderLink, &link) != nil {
			return wecomapp.CustomerAcquisitionLinkReceipt{}, errors.New("invalid customer acquisition link receipt provider link")
		}
		receipt.Link = &link
	}
	return receipt, nil
}

func validCustomerAcquisitionLinkReceiptCompletion(completion wecomapp.CustomerAcquisitionLinkCompletion) bool {
	switch completion.State {
	case wecomapp.CustomerAcquisitionLinkExecuted, wecomapp.CustomerAcquisitionLinkFinalFailed, wecomapp.CustomerAcquisitionLinkOutcomeUnknown:
		return completion.ReconcileActor == 0 && completion.ReconcileKeyDigest == ([32]byte{}) && completion.EvidenceDigest == ([32]byte{}) && completion.Resolution == ""
	case wecomapp.CustomerAcquisitionLinkReconciled:
		return completion.BusinessEndpointDispatched && completion.RealExternalCallExecuted && completion.ReconcileActor > 0 && completion.ReconcileKeyDigest != ([32]byte{}) && completion.EvidenceDigest != ([32]byte{}) && (completion.Resolution == wecomapp.CustomerAcquisitionLinkProviderApplied || completion.Resolution == wecomapp.CustomerAcquisitionLinkProviderNotApplied)
	default:
		return false
	}
}

func customerAcquisitionLinkReceiptStoreError(err error) error {
	if errors.Is(err, wecomapp.ErrCustomerAcquisitionLinkUnavailable) {
		return err
	}
	return fmt.Errorf("%w: %v", wecomapp.ErrCustomerAcquisitionLinkUnavailable, err)
}
