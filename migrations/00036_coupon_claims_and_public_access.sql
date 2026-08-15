-- +goose Up
ALTER TABLE public.coupons DROP CONSTRAINT coupons_status;
ALTER TABLE public.coupons ADD CONSTRAINT coupons_status CHECK (status IN ('draft', 'published', 'stopped', 'archived'));

ALTER TABLE public.coupon_operation_receipts DROP CONSTRAINT coupon_receipts_operation;
ALTER TABLE public.coupon_operation_receipts ADD CONSTRAINT coupon_receipts_operation CHECK (operation IN ('create','update','publish','stop','archive','delete','copy','claim'));

CREATE TABLE public.coupon_claims (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  coupon_id BIGINT NOT NULL REFERENCES public.coupons(id) ON DELETE RESTRICT,
  customer_id BIGINT NOT NULL,
  claim_number INTEGER NOT NULL,
  claim_ref TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'claimed',
  claimed_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT coupon_claims_customer CHECK (customer_id > 0),
  CONSTRAINT coupon_claims_number CHECK (claim_number > 0),
  CONSTRAINT coupon_claims_ref CHECK (claim_ref ~ '^cp_[A-Za-z0-9_-]{16,64}$'),
  CONSTRAINT coupon_claims_status CHECK (status = 'claimed'),
  UNIQUE (coupon_id, customer_id, claim_number),
  UNIQUE (claim_ref)
);
CREATE INDEX coupon_claims_coupon_id_id ON public.coupon_claims (coupon_id, id);
CREATE INDEX coupon_claims_customer_coupon_id ON public.coupon_claims (customer_id, coupon_id, id);
CREATE INDEX coupon_targets_target_ref_coupon_id ON public.coupon_targets (target_ref, coupon_id);

-- An opaque session is issued only by a trusted payment-identity/OAuth flow.
-- The coupon transport receives only its raw browser token; persistence stores
-- a digest and an already-resolved OneID, never caller-supplied identity data.
CREATE TABLE public.coupon_payment_identity_sessions (
  token_digest BYTEA PRIMARY KEY,
  customer_id BIGINT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  replaced_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT coupon_payment_identity_token CHECK (octet_length(token_digest) = 32),
  CONSTRAINT coupon_payment_identity_customer CHECK (customer_id > 0),
  CONSTRAINT coupon_payment_identity_lifecycle CHECK (expires_at > created_at AND (revoked_at IS NULL OR revoked_at >= created_at) AND (replaced_at IS NULL OR replaced_at >= created_at))
);
CREATE INDEX coupon_payment_identity_active_lookup ON public.coupon_payment_identity_sessions (expires_at, customer_id) WHERE revoked_at IS NULL AND replaced_at IS NULL;

-- Sidebar grants are distinct from payment-identity sessions. Both are opaque
-- server-side credentials, but a sidebar grant may only select its owner for a
-- sensitive read and cannot be presented to claim a coupon.
CREATE TABLE public.coupon_sidebar_grants (
  token_digest BYTEA PRIMARY KEY,
  customer_id BIGINT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  replaced_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT coupon_sidebar_grant_token CHECK (octet_length(token_digest) = 32),
  CONSTRAINT coupon_sidebar_grant_customer CHECK (customer_id > 0),
  CONSTRAINT coupon_sidebar_grant_lifecycle CHECK (expires_at > created_at AND (revoked_at IS NULL OR revoked_at >= created_at) AND (replaced_at IS NULL OR replaced_at >= created_at))
);
CREATE INDEX coupon_sidebar_grant_active_lookup ON public.coupon_sidebar_grants (expires_at, customer_id) WHERE revoked_at IS NULL AND replaced_at IS NULL;

-- +goose Down
DROP INDEX public.coupon_sidebar_grant_active_lookup;
DROP TABLE public.coupon_sidebar_grants;
DROP INDEX public.coupon_payment_identity_active_lookup;
DROP TABLE public.coupon_payment_identity_sessions;
DROP INDEX public.coupon_targets_target_ref_coupon_id;
DROP INDEX public.coupon_claims_customer_coupon_id;
DROP INDEX public.coupon_claims_coupon_id_id;
DROP TABLE public.coupon_claims;
ALTER TABLE public.coupon_operation_receipts DROP CONSTRAINT coupon_receipts_operation;
ALTER TABLE public.coupon_operation_receipts ADD CONSTRAINT coupon_receipts_operation CHECK (operation IN ('create','update','publish','stop','archive','delete','copy','claim'));
ALTER TABLE public.coupons DROP CONSTRAINT coupons_status;
ALTER TABLE public.coupons ADD CONSTRAINT coupons_status CHECK (status IN ('draft', 'published', 'stopped', 'archived'));
