-- Execute manually as postgres in photo_search.
-- This table contains authenticated share references, never application credentials.
BEGIN;
CREATE TABLE IF NOT EXISTS public.creation_shares (
 id text PRIMARY KEY,
 creation_id text NOT NULL UNIQUE REFERENCES public.creations(id) ON DELETE CASCADE,
 version_id text NOT NULL REFERENCES public.creation_versions(id) ON DELETE CASCADE,
 enabled boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL
);
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.creation_shares TO photo_user;
COMMIT;
