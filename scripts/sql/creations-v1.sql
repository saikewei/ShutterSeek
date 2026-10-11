-- Manual migration. Do not run against production without user approval.
BEGIN;
CREATE TABLE creations (
 id text PRIMARY KEY, user_id bigint NOT NULL REFERENCES users(id),
 title text NOT NULL, photo_ids text NOT NULL, state text NOT NULL DEFAULT '{}',
 current_version text NOT NULL DEFAULT '', created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
CREATE INDEX creations_owner ON creations(user_id, created_at DESC);
CREATE TABLE creation_messages (
 id text PRIMARY KEY, creation_id text NOT NULL REFERENCES creations(id),
 role text NOT NULL CHECK(role IN ('user','assistant')), content text NOT NULL, created_at timestamptz NOT NULL
);
CREATE TABLE creation_runs (
 id text PRIMARY KEY, creation_id text NOT NULL REFERENCES creations(id), user_id bigint NOT NULL REFERENCES users(id),
 base_version text NOT NULL, action text NOT NULL, status text NOT NULL CHECK(status IN ('queued','running','waiting','completed','cancelled','failed')),
 credential_hash text NOT NULL DEFAULT '', expires_at timestamptz NOT NULL, report text NOT NULL DEFAULT '{}',
 input text NOT NULL, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX creation_active_user ON creation_runs(user_id) WHERE status IN ('queued','running');
CREATE UNIQUE INDEX creation_active_work ON creation_runs(creation_id) WHERE status IN ('queued','running');
CREATE TABLE creation_versions (
 id text PRIMARY KEY, creation_id text NOT NULL REFERENCES creations(id), run_id text NOT NULL UNIQUE REFERENCES creation_runs(id),
 html text NOT NULL, state text NOT NULL, report text NOT NULL, used_photo_ids text NOT NULL, verified boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL
);
CREATE TABLE creation_events (
 id bigserial PRIMARY KEY, run_id text NOT NULL REFERENCES creation_runs(id), data text NOT NULL, created_at timestamptz NOT NULL
);
CREATE INDEX creation_event_replay ON creation_events(run_id,id);
CREATE TABLE creation_assets (
 id text PRIMARY KEY, run_id text NOT NULL REFERENCES creation_runs(id), data bytea NOT NULL, metadata text NOT NULL, created_at timestamptz NOT NULL
);
CREATE TABLE creation_checkpoints (
 id text PRIMARY KEY, run_id text NOT NULL REFERENCES creation_runs(id), payload text NOT NULL, updated_at timestamptz NOT NULL
);

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE
 public.creations,
 public.creation_messages,
 public.creation_runs,
 public.creation_versions,
 public.creation_events,
 public.creation_assets,
 public.creation_checkpoints
TO photo_user;
GRANT USAGE, SELECT ON SEQUENCE public.creation_events_id_seq TO photo_user;
COMMIT;
