package contactfixture

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidCampaignDispatchFixture = errors.New("invalid campaign dispatch contact fixture")

type CampaignDispatchFacts struct {
	SuppressedCustomerID int64
	EligibleCustomerID   int64
	UnresolvedCustomerID int64
}

func CreateCampaignDispatchFacts(ctx context.Context, pool *pgxpool.Pool, ownerWeComUserID string, at time.Time) (CampaignDispatchFacts, error) {
	if ctx == nil || pool == nil || ownerWeComUserID == "" || at.IsZero() {
		return CampaignDispatchFacts{}, ErrInvalidCampaignDispatchFixture
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return CampaignDispatchFacts{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ownerID, err := CreateStaffWithDetails(ctx, tx, ownerWeComUserID, "dispatch owner", true, at)
	if err != nil {
		return CampaignDispatchFacts{}, err
	}
	result := CampaignDispatchFacts{}
	customers := []struct {
		destination *int64
		name        string
	}{
		{destination: &result.SuppressedCustomerID, name: "suppressed after preview"},
		{destination: &result.EligibleCustomerID, name: "eligible one"},
		{destination: &result.UnresolvedCustomerID, name: "eligible two"},
	}
	for _, customer := range customers {
		if err = tx.QueryRow(ctx, `INSERT INTO customers(name,is_deleted) VALUES($1,FALSE) RETURNING id`, customer.name).Scan(customer.destination); err != nil {
			return CampaignDispatchFacts{}, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE customers SET owner_staff_id=$2 WHERE id=$1`, result.EligibleCustomerID, ownerID); err != nil {
		return CampaignDispatchFacts{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO customer_contact_policies(customer_id,reason_code,created_at,updated_at) VALUES($1,'manual_opt_out',$2,$2)`, result.SuppressedCustomerID, at); err != nil {
		return CampaignDispatchFacts{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CampaignDispatchFacts{}, err
	}
	return result, nil
}

func DeleteCampaignDispatchPolicy(ctx context.Context, pool *pgxpool.Pool, customerID int64) error {
	if ctx == nil || pool == nil || customerID < 1 {
		return ErrInvalidCampaignDispatchFixture
	}
	_, err := pool.Exec(ctx, `DELETE FROM customer_contact_policies WHERE customer_id=$1`, customerID)
	return err
}
