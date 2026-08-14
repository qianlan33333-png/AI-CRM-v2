-- +goose Up
CREATE TABLE public.coupons (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL,
  discount_amount_total BIGINT NOT NULL,
  currency CHAR(3) NOT NULL DEFAULT 'CNY',
  status TEXT NOT NULL DEFAULT 'draft',
  total_issue_limit BIGINT NOT NULL,
  per_user_issue_limit BIGINT NOT NULL DEFAULT 1,
  issued_count BIGINT NOT NULL DEFAULT 0,
  claim_starts_at TIMESTAMPTZ NOT NULL,
  claim_ends_at TIMESTAMPTZ NOT NULL,
  validity_mode TEXT NOT NULL,
  use_starts_at TIMESTAMPTZ,
  use_ends_at TIMESTAMPTZ,
  relative_validity_days INTEGER,
  instructions TEXT NOT NULL DEFAULT '',
  first_claim_at TIMESTAMPTZ,
  created_by BIGINT NOT NULL,
  updated_by BIGINT NOT NULL,
  version BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT coupons_name CHECK (btrim(name) = name AND name <> '' AND char_length(name) <= 45),
  CONSTRAINT coupons_discount CHECK (discount_amount_total > 0),
  CONSTRAINT coupons_currency CHECK (currency = 'CNY'),
  CONSTRAINT coupons_status CHECK (status IN ('draft', 'published', 'stopped')),
  CONSTRAINT coupons_quantities CHECK (total_issue_limit > 0 AND per_user_issue_limit > 0 AND per_user_issue_limit <= total_issue_limit AND issued_count BETWEEN 0 AND total_issue_limit),
  CONSTRAINT coupons_claim_range CHECK (claim_ends_at > claim_starts_at),
  CONSTRAINT coupons_validity CHECK (
    (validity_mode = 'fixed_range' AND use_starts_at IS NOT NULL AND use_ends_at > use_starts_at AND relative_validity_days IS NULL) OR
    (validity_mode = 'relative_days' AND use_starts_at IS NULL AND use_ends_at IS NULL AND relative_validity_days > 0)
  ),
  CONSTRAINT coupons_instructions CHECK (btrim(instructions) = instructions AND char_length(instructions) <= 200),
  CONSTRAINT coupons_claim_fact CHECK ((issued_count = 0 AND first_claim_at IS NULL) OR (issued_count > 0 AND first_claim_at IS NOT NULL)),
  CONSTRAINT coupons_actors CHECK (created_by > 0 AND updated_by > 0),
  CONSTRAINT coupons_version CHECK (version > 0),
  CONSTRAINT coupons_timestamps CHECK (updated_at >= created_at)
);
CREATE INDEX coupons_list_id ON public.coupons (id);
CREATE INDEX coupons_list_status_id ON public.coupons (status, id);
CREATE INDEX coupons_list_name_pattern ON public.coupons (name text_pattern_ops, id);

CREATE TABLE public.coupon_targets (
  coupon_id BIGINT NOT NULL REFERENCES public.coupons(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  target_ref TEXT NOT NULL,
  product_id BIGINT NOT NULL,
  PRIMARY KEY (coupon_id, position),
  UNIQUE (coupon_id, target_ref),
  CONSTRAINT coupon_targets_position CHECK (position >= 0),
  CONSTRAINT coupon_targets_ref CHECK (target_ref ~ '^standard_product:[1-9][0-9]*$'),
  CONSTRAINT coupon_targets_product CHECK (product_id > 0)
);

CREATE TABLE public.coupon_catalog_counters (
  singleton BOOLEAN PRIMARY KEY DEFAULT TRUE,
  total_coupons BIGINT NOT NULL DEFAULT 0,
  CONSTRAINT coupon_catalog_counters_singleton CHECK (singleton),
  CONSTRAINT coupon_catalog_counters_total CHECK (total_coupons >= 0)
);
INSERT INTO public.coupon_catalog_counters (singleton, total_coupons) VALUES (TRUE, 0);

CREATE TABLE public.coupon_operation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL,
  actor_scope TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'in_progress',
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT coupon_receipts_operation CHECK (operation IN ('create','update','publish','stop')),
  CONSTRAINT coupon_receipts_actor CHECK (btrim(actor_scope) = actor_scope AND actor_scope <> '' AND char_length(actor_scope) <= 200),
  CONSTRAINT coupon_receipts_key CHECK (octet_length(key_digest) = 32),
  CONSTRAINT coupon_receipts_payload CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT coupon_receipts_state CHECK (state IN ('in_progress','completed')),
  CONSTRAINT coupon_receipts_completion CHECK ((state = 'in_progress' AND result_snapshot IS NULL AND completed_at IS NULL) OR (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)),
  UNIQUE (operation, actor_scope, key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_coupon_receipt_complete_before_commit()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM public.coupon_operation_receipts WHERE id = NEW.id AND state = 'completed') THEN
    RAISE EXCEPTION 'coupon operation receipt must complete in reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER coupon_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.coupon_operation_receipts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_coupon_receipt_complete_before_commit();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_coupon_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed coupon receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.operation IS DISTINCT FROM OLD.operation OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope
     OR NEW.key_digest IS DISTINCT FROM OLD.key_digest OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid coupon receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER coupon_receipts_transition BEFORE UPDATE OR DELETE ON public.coupon_operation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_coupon_receipt_transition_valid();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_coupon_claimed_rules_frozen()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF OLD.issued_count > 0 AND (
    NEW.name IS DISTINCT FROM OLD.name OR NEW.discount_amount_total IS DISTINCT FROM OLD.discount_amount_total OR
    NEW.currency IS DISTINCT FROM OLD.currency OR NEW.per_user_issue_limit IS DISTINCT FROM OLD.per_user_issue_limit OR
    NEW.claim_starts_at IS DISTINCT FROM OLD.claim_starts_at OR NEW.claim_ends_at IS DISTINCT FROM OLD.claim_ends_at OR
    NEW.validity_mode IS DISTINCT FROM OLD.validity_mode OR NEW.use_starts_at IS DISTINCT FROM OLD.use_starts_at OR
    NEW.use_ends_at IS DISTINCT FROM OLD.use_ends_at OR NEW.relative_validity_days IS DISTINCT FROM OLD.relative_validity_days OR
    NEW.instructions IS DISTINCT FROM OLD.instructions OR NEW.total_issue_limit < OLD.total_issue_limit
  ) THEN RAISE EXCEPTION 'claimed coupon rules are frozen' USING ERRCODE = '55000'; END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER coupons_claimed_rules_frozen BEFORE UPDATE ON public.coupons
FOR EACH ROW EXECUTE FUNCTION public.aicrm_coupon_claimed_rules_frozen();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_coupon_claimed_targets_frozen()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE target_coupon_id BIGINT;
BEGIN
  target_coupon_id := CASE WHEN TG_OP = 'DELETE' THEN OLD.coupon_id ELSE NEW.coupon_id END;
  IF EXISTS (SELECT 1 FROM public.coupons WHERE id = target_coupon_id AND issued_count > 0) THEN
    RAISE EXCEPTION 'claimed coupon targets are frozen' USING ERRCODE = '55000';
  END IF;
  RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER coupon_targets_claimed_frozen BEFORE INSERT OR UPDATE OR DELETE ON public.coupon_targets
FOR EACH ROW EXECUTE FUNCTION public.aicrm_coupon_claimed_targets_frozen();

-- +goose Down
DROP TRIGGER coupon_targets_claimed_frozen ON public.coupon_targets;
DROP FUNCTION public.aicrm_coupon_claimed_targets_frozen();
DROP TRIGGER coupons_claimed_rules_frozen ON public.coupons;
DROP FUNCTION public.aicrm_coupon_claimed_rules_frozen();
DROP TRIGGER coupon_receipts_transition ON public.coupon_operation_receipts;
DROP TRIGGER coupon_receipts_complete_before_commit ON public.coupon_operation_receipts;
DROP TABLE public.coupon_operation_receipts;
DROP TABLE public.coupon_catalog_counters;
DROP TABLE public.coupon_targets;
DROP TABLE public.coupons;
DROP FUNCTION public.aicrm_coupon_receipt_transition_valid();
DROP FUNCTION public.aicrm_coupon_receipt_complete_before_commit();
