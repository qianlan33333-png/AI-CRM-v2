-- +goose Up
CREATE INDEX identities_verified_wecom_customer_scope_idx
  ON public.identities(customer_id, scope, normalized_value)
  WHERE kind = 'wecom_external_userid'
    AND assurance = 'verified'
    AND customer_id IS NOT NULL
    AND bound_at IS NOT NULL;

-- +goose Down
DROP INDEX public.identities_verified_wecom_customer_scope_idx;
