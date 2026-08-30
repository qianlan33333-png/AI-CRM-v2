CREATE OR REPLACE FUNCTION public.aicrm_ai_audience_configuration_version_immutable()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
  RAISE EXCEPTION 'AI Audience configuration versions are immutable'
    USING ERRCODE = '55000';
END;
$function$
;
CREATE OR REPLACE FUNCTION public.aicrm_ai_audience_local_configuration_receipt_complete_before_c()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM public.ai_audience_local_configuration_receipts
    WHERE id = NEW.id
      AND state = 'completed'
      AND result_json IS NOT NULL
      AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'AI Audience local configuration receipt must complete in its reservation transaction'
      USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.aicrm_ai_audience_local_configuration_receipt_transition()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed AI Audience local configuration receipts are immutable'
      USING ERRCODE = '55000';
  END IF;

  IF NEW.state <> 'completed'
     OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_id IS DISTINCT FROM OLD.actor_id
     OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest
     OR NEW.created_at IS DISTINCT FROM OLD.created_at
     OR NEW.result_json IS NULL
     OR NEW.completed_at IS NULL THEN
    RAISE EXCEPTION 'invalid AI Audience local configuration receipt transition'
      USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.aicrm_ai_audience_package_activation_binding_conflict()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
DECLARE
  selected_agent_id BIGINT;
BEGIN
  IF OLD.lifecycle = 'archived' AND NEW.lifecycle <> 'archived' THEN
    SELECT binding.automation_agent_id
      INTO selected_agent_id
    FROM public.ai_audience_package_automation_bindings AS binding
    WHERE binding.package_id = NEW.segment_id;

    IF selected_agent_id IS NOT NULL THEN
      PERFORM pg_advisory_xact_lock(
        hashtextextended(
          'ai_audience.package.automation_binding.v1:' || selected_agent_id::text,
          0
        )
      );
      IF EXISTS (
        SELECT 1
        FROM public.ai_audience_package_automation_bindings AS binding
        JOIN public.ai_audience_package_metadata AS metadata
          ON metadata.segment_id = binding.package_id
        WHERE binding.automation_agent_id = selected_agent_id
          AND binding.package_id <> NEW.segment_id
          AND metadata.lifecycle <> 'archived'
      ) THEN
        RAISE EXCEPTION 'automation agent is already bound by a non-archived AI Audience package'
          USING ERRCODE = '23505';
      END IF;
    END IF;
  END IF;
  RETURN NEW;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.aicrm_channel_receipt_transition_valid()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed channel operation receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid channel operation receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.aicrm_entitlement_receipt_transition_valid()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed entitlement operation receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid entitlement operation receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.aicrm_external_effects_reject_delete()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
  RAISE EXCEPTION 'external effect facts cannot be deleted' USING ERRCODE = '55000';
END $function$
;

CREATE OR REPLACE FUNCTION public.aicrm_order_list_projection_count_delete()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
DECLARE deleted_count BIGINT;
BEGIN
  SELECT count(*) INTO deleted_count FROM deleted_rows;
  UPDATE public.order_list_projection_counters SET total_orders = total_orders - deleted_count WHERE singleton = TRUE;
  RETURN NULL;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.aicrm_order_list_projection_count_insert()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
DECLARE inserted_count BIGINT;
BEGIN
  SELECT count(*) INTO inserted_count FROM inserted_rows;
  UPDATE public.order_list_projection_counters SET total_orders = total_orders + inserted_count WHERE singleton = TRUE;
  RETURN NULL;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.aicrm_order_receipt_transition_valid()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed order operation receipt is immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid order operation receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.aicrm_product_receipt_transition_valid()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed product operation receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid product operation receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.aicrm_questionnaire_submission_count_sync()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
  IF TG_OP = 'INSERT' THEN
    UPDATE public.questionnaires SET submission_count = submission_count + 1 WHERE id = NEW.questionnaire_id;
    RETURN NEW;
  END IF;
  IF TG_OP = 'DELETE' THEN
    UPDATE public.questionnaires SET submission_count = submission_count - 1
    WHERE id = OLD.questionnaire_id AND submission_count > 0;
    RETURN OLD;
  END IF;
  RETURN NULL;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.aicrm_questionnaire_submission_immutable()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
  RAISE EXCEPTION 'questionnaire submission snapshots are immutable' USING ERRCODE = '55000';
END;
$function$
;

CREATE OR REPLACE FUNCTION public.aicrm_reject_incomplete_channel_receipt()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM public.channel_operation_receipts WHERE id = NEW.id AND state = 'completed') THEN
    RAISE EXCEPTION 'channel operation receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.aicrm_reject_incomplete_entitlement_receipt()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.entitlement_operation_receipts
    WHERE id = NEW.id AND state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'entitlement operation receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.aicrm_reject_incomplete_order_receipt()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM public.order_operation_receipts WHERE id = NEW.id AND state = 'completed') THEN
    RAISE EXCEPTION 'order operation receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.aicrm_reject_incomplete_product_receipt()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.product_operation_receipts
    WHERE id = NEW.id AND state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'product operation receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.aicrm_reject_incomplete_segment_receipt()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM public.segment_operation_receipts
    WHERE id = NEW.id
      AND state = 'completed'
      AND result_segment_id IS NOT NULL
      AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'segment operation receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.aicrm_segment_receipt_transition_valid()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed segment operation receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed'
     OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope
     OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid segment operation receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.aicrm_service_period_member_receipt_complete_before_commit()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.service_period_member_operation_receipts
    WHERE id = NEW.id AND state = 'completed'
      AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'service-period member receipt must complete in its reservation transaction'
      USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.aicrm_service_period_member_receipt_transition_valid()
 RETURNS trigger
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed service-period member receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid service-period member receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.radar_links_reject_public_code_change()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
BEGIN
    IF NEW.public_code IS DISTINCT FROM OLD.public_code THEN
        RAISE EXCEPTION 'radar_links.public_code is immutable'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.river_job_state_in_bitmask(bitmask bit, state river_job_state)
 RETURNS boolean
 LANGUAGE sql
 IMMUTABLE
AS $function$
    SELECT CASE state
        WHEN 'available' THEN get_bit(bitmask, 7)
        WHEN 'cancelled' THEN get_bit(bitmask, 6)
        WHEN 'completed' THEN get_bit(bitmask, 5)
        WHEN 'discarded' THEN get_bit(bitmask, 4)
        WHEN 'pending'   THEN get_bit(bitmask, 3)
        WHEN 'retryable' THEN get_bit(bitmask, 2)
        WHEN 'running'   THEN get_bit(bitmask, 1)
        WHEN 'scheduled' THEN get_bit(bitmask, 0)
        ELSE 0
    END = 1;
$function$
;
