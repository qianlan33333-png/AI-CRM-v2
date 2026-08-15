-- +goose Up
-- Operation-cycle state is local to one private deployment.  It stores no
-- provider credentials, raw conversations, paths, or deployment selectors.
CREATE TABLE public.operation_cycle_strategies (
  strategy_key TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  version INTEGER NOT NULL,
  definition JSONB NOT NULL,
  snapshot JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT operation_cycle_strategies_key CHECK (strategy_key ~ '^[A-Za-z0-9][A-Za-z0-9_.:-]{0,119}$'),
  CONSTRAINT operation_cycle_strategies_title CHECK (length(title) BETWEEN 1 AND 200),
  CONSTRAINT operation_cycle_strategies_status CHECK (status IN ('active', 'paused', 'archived', 'draft')),
  CONSTRAINT operation_cycle_strategies_version CHECK (version > 0)
);

CREATE TABLE public.operation_cycle_runs (
  run_key TEXT PRIMARY KEY,
  strategy_key TEXT NOT NULL REFERENCES public.operation_cycle_strategies(strategy_key) ON DELETE RESTRICT,
  snapshot_revision INTEGER NOT NULL,
  snapshot JSONB NOT NULL,
  received_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT operation_cycle_runs_key CHECK (run_key ~ '^[A-Za-z0-9][A-Za-z0-9_.:-]{0,159}$'),
  CONSTRAINT operation_cycle_runs_revision CHECK (snapshot_revision > 0)
);
CREATE INDEX operation_cycle_runs_strategy_received ON public.operation_cycle_runs (strategy_key, received_at DESC, run_key DESC);

CREATE TABLE public.operation_cycle_report_receipts (
  id TEXT PRIMARY KEY,
  actor_scope TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  strategy_key TEXT NOT NULL REFERENCES public.operation_cycle_strategies(strategy_key) ON DELETE RESTRICT,
  run_key TEXT NOT NULL REFERENCES public.operation_cycle_runs(run_key) ON DELETE RESTRICT,
  accepted_revision INTEGER NOT NULL,
  projection_made BOOLEAN NOT NULL,
  CONSTRAINT operation_cycle_report_receipts_key_digest CHECK (octet_length(key_digest) = 32),
  CONSTRAINT operation_cycle_report_receipts_payload_digest CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT operation_cycle_report_receipts_revision CHECK (accepted_revision > 0),
  UNIQUE (actor_scope, key_digest)
);

CREATE TABLE public.operation_cycle_runners (
  runner_id TEXT PRIMARY KEY,
  principal_id TEXT NOT NULL,
  connector_version TEXT NOT NULL,
  codex_version TEXT NOT NULL,
  compatibility_status TEXT NOT NULL,
  binding_keys JSONB NOT NULL,
  last_heartbeat_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT operation_cycle_runners_id CHECK (runner_id ~ '^[A-Za-z0-9][A-Za-z0-9_.:-]{0,159}$'),
  CONSTRAINT operation_cycle_runners_principal CHECK (principal_id ~ '^[A-Za-z0-9][A-Za-z0-9_.:-]{0,239}$'),
  CONSTRAINT operation_cycle_runners_status CHECK (compatibility_status IN ('ready', 'incompatible', 'unavailable'))
);
CREATE INDEX operation_cycle_runners_ready_heartbeat ON public.operation_cycle_runners (last_heartbeat_at, runner_id) WHERE compatibility_status = 'ready';

CREATE TABLE public.operation_cycle_action_requests (
  request_id TEXT PRIMARY KEY,
  strategy_key TEXT NOT NULL REFERENCES public.operation_cycle_strategies(strategy_key) ON DELETE RESTRICT,
  run_key TEXT NOT NULL REFERENCES public.operation_cycle_runs(run_key) ON DELETE RESTRICT,
  action_key TEXT NOT NULL,
  action_title TEXT NOT NULL,
  strategy_version INTEGER NOT NULL,
  runner_id TEXT NOT NULL REFERENCES public.operation_cycle_runners(runner_id) ON DELETE RESTRICT,
  status TEXT NOT NULL,
  parent_request_id TEXT,
  thread_id TEXT,
  turn_id TEXT,
  final_result JSONB,
  failure_code TEXT,
  created_by TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  idempotency_key_digest BYTEA NOT NULL,
  lease_token_hash BYTEA,
  lease_expires_at TIMESTAMPTZ,
  CONSTRAINT operation_cycle_action_requests_id CHECK (request_id ~ '^ocact_[a-f0-9]{28}$'),
  CONSTRAINT operation_cycle_action_requests_status CHECK (status IN ('queued', 'claimed', 'thread_bound', 'turn_started', 'completed', 'failed')),
  CONSTRAINT operation_cycle_action_requests_version CHECK (strategy_version > 0),
  CONSTRAINT operation_cycle_action_requests_idempotency_digest CHECK (octet_length(idempotency_key_digest) = 32),
  CONSTRAINT operation_cycle_action_requests_lease_digest CHECK (lease_token_hash IS NULL OR octet_length(lease_token_hash) = 32),
  CONSTRAINT operation_cycle_action_requests_lifecycle CHECK (
    (status IN ('queued', 'claimed', 'thread_bound', 'turn_started') AND completed_at IS NULL) OR
    (status IN ('completed', 'failed') AND completed_at IS NOT NULL)
  ),
  UNIQUE (idempotency_key_digest)
);
CREATE UNIQUE INDEX operation_cycle_one_active_action_per_strategy ON public.operation_cycle_action_requests (strategy_key) WHERE status IN ('queued', 'claimed', 'thread_bound', 'turn_started');
CREATE INDEX operation_cycle_action_claim_queue ON public.operation_cycle_action_requests (runner_id, created_at, request_id) WHERE status = 'queued';

CREATE TABLE public.operation_cycle_action_request_events (
  request_id TEXT NOT NULL REFERENCES public.operation_cycle_action_requests(request_id) ON DELETE RESTRICT,
  event_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  payload_digest BYTEA NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (request_id, event_id),
  CONSTRAINT operation_cycle_action_request_events_type CHECK (event_type IN ('thread_bound', 'turn_started', 'completed', 'failed')),
  CONSTRAINT operation_cycle_action_request_events_digest CHECK (octet_length(payload_digest) = 32)
);

CREATE TABLE public.operation_cycle_strategy_proposals (
  proposal_id TEXT PRIMARY KEY,
  strategy_key TEXT NOT NULL REFERENCES public.operation_cycle_strategies(strategy_key) ON DELETE RESTRICT,
  base_strategy_version INTEGER NOT NULL,
  status TEXT NOT NULL,
  proposal JSONB NOT NULL,
  created_by TEXT NOT NULL,
  decided_by TEXT,
  created_at TIMESTAMPTZ NOT NULL,
  decided_at TIMESTAMPTZ,
  idempotency_key_digest BYTEA NOT NULL,
  CONSTRAINT operation_cycle_strategy_proposals_id CHECK (proposal_id ~ '^ocprop_[a-f0-9]{28}$'),
  CONSTRAINT operation_cycle_strategy_proposals_version CHECK (base_strategy_version > 0),
  CONSTRAINT operation_cycle_strategy_proposals_status CHECK (status IN ('pending', 'accepted', 'rejected')),
  CONSTRAINT operation_cycle_strategy_proposals_idempotency_digest CHECK (octet_length(idempotency_key_digest) = 32),
  CONSTRAINT operation_cycle_strategy_proposals_decision CHECK ((status = 'pending' AND decided_by IS NULL AND decided_at IS NULL) OR (status IN ('accepted', 'rejected') AND decided_by IS NOT NULL AND decided_at IS NOT NULL)),
  UNIQUE (created_by, idempotency_key_digest)
);
CREATE INDEX operation_cycle_strategy_proposals_list ON public.operation_cycle_strategy_proposals (strategy_key, created_at DESC, proposal_id DESC);

-- +goose Down
DROP INDEX public.operation_cycle_strategy_proposals_list;
DROP TABLE public.operation_cycle_strategy_proposals;
DROP TABLE public.operation_cycle_action_request_events;
DROP INDEX public.operation_cycle_action_claim_queue;
DROP INDEX public.operation_cycle_one_active_action_per_strategy;
DROP TABLE public.operation_cycle_action_requests;
DROP INDEX public.operation_cycle_runners_ready_heartbeat;
DROP TABLE public.operation_cycle_runners;
DROP TABLE public.operation_cycle_report_receipts;
DROP INDEX public.operation_cycle_runs_strategy_received;
DROP TABLE public.operation_cycle_runs;
DROP TABLE public.operation_cycle_strategies;
