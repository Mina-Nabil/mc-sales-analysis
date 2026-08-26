-- User management (§11.5): every user is an admin — no roles — but a user can be
-- deactivated (blocked from every request) or deleted.

-- is_active: deactivated users fail authentication, so they cannot make requests.
ALTER TABLE users ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT true;

-- is_seed marks the bootstrap admin created by `seed-admin`. It is the one
-- account that can never be deleted or deactivated, so the system can always be
-- logged into. The earliest existing user is the bootstrap account.
ALTER TABLE users ADD COLUMN is_seed BOOLEAN NOT NULL DEFAULT false;
UPDATE users SET is_seed = true WHERE id = (SELECT MIN(id) FROM users);

-- Deleting a user must NULL their attribution rather than fail or cascade: the
-- audit trail and history stay, they just lose the actor reference. (sessions
-- keeps ON DELETE CASCADE — a deleted user's sessions must die with them.)
ALTER TABLE brand_aliases  DROP CONSTRAINT brand_aliases_decided_by_fkey,
  ADD CONSTRAINT brand_aliases_decided_by_fkey
  FOREIGN KEY (decided_by) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE model_aliases  DROP CONSTRAINT model_aliases_decided_by_fkey,
  ADD CONSTRAINT model_aliases_decided_by_fkey
  FOREIGN KEY (decided_by) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE import_batches DROP CONSTRAINT import_batches_created_by_fkey,
  ADD CONSTRAINT import_batches_created_by_fkey
  FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE change_log     DROP CONSTRAINT change_log_actor_id_fkey,
  ADD CONSTRAINT change_log_actor_id_fkey
  FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE SET NULL;
