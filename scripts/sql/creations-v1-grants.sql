-- Manual permission repair for installations created by an administrative role.
-- Execute in photo_search as the table owner or another authorized role.
BEGIN;
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
