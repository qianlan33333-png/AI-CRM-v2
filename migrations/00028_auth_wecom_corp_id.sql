-- +goose Up
ALTER TABLE admin_users
  RENAME COLUMN provider_tenant_id TO wecom_corp_id;

ALTER TABLE admin_users
  RENAME CONSTRAINT ck_admin_users_provider_tenant TO ck_admin_users_wecom_corp_id;

-- PostgreSQL renames the UNIQUE constraint's backing index with the constraint.
ALTER TABLE admin_users
  RENAME CONSTRAINT uq_admin_users_provider_identity TO uq_admin_users_wecom_identity;

-- +goose Down
ALTER TABLE admin_users
  RENAME CONSTRAINT uq_admin_users_wecom_identity TO uq_admin_users_provider_identity;

ALTER TABLE admin_users
  RENAME CONSTRAINT ck_admin_users_wecom_corp_id TO ck_admin_users_provider_tenant;

ALTER TABLE admin_users
  RENAME COLUMN wecom_corp_id TO provider_tenant_id;
