-- Manual migration for ShutterSeek vector search optimization (Phase 1).
-- Do NOT run automatically; execute manually as the table owner (postgres):
--
--   docker exec -i postgres-main psql -U postgres -d photo_search -v ON_ERROR_STOP=1 \
--     < scripts/sql/search-optimization-v1.sql
--
-- Requirements:
--   - PostgreSQL 16+ with pgvector >= 0.8.0 (verified on 0.8.5)
--   - CREATE INDEX CONCURRENTLY statements MUST run outside a transaction block.
--
-- What this migration does:
--   1. Adds compact halfvec(1024) + denormalized is_public & taken_at columns to
--      photo_embeddings (source embeddings were generated in fp16; halfvec cuts
--      index size in half with 100% top-20 recall and <2e-6 cosine delta).
--   2. Installs lightweight triggers so photo_embeddings stays synchronized on
--      photo uploads, taken_at edits, album membership changes, and album
--      is_public toggles.
--   3. Backfills existing rows.
--   4. Builds CONCURRENTLY:
--        - idx_pe_hnsw_public_hv (~75-85 MB partial HNSW for public photos)
--        - idx_pe_hnsw_all_hv    (~240 MB full-library halfvec HNSW)
--        - idx_pe_public_taken   (B-Tree for public + time-range pre-filtering)
--        - idx_pe_taken_photo    (B-Tree for admin time-range pre-filtering)
--        - idx_albums_public     (Partial B-Tree for public albums)
--   5. Runs VACUUM ANALYZE on search tables.

-- ============================================================================
-- Step 1: Columns and synchronization triggers (transactional)
-- ============================================================================
BEGIN;

ALTER TABLE public.photo_embeddings
  ADD COLUMN IF NOT EXISTS embedding_hv halfvec(1024),
  ADD COLUMN IF NOT EXISTS is_public boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS taken_at timestamptz;

CREATE OR REPLACE FUNCTION public.sync_photo_embedding_visibility(target_photo_id bigint)
RETURNS void
LANGUAGE sql
AS $$
  UPDATE public.photo_embeddings pe
  SET is_public = sub.should_be_public
  FROM (
    SELECT EXISTS (
      SELECT 1
      FROM public.album_photos ap
      JOIN public.albums a ON a.id = ap.album_id
      WHERE ap.photo_id = target_photo_id
        AND a.is_public = true
    ) AS should_be_public
  ) sub
  WHERE pe.photo_id = target_photo_id
    AND pe.is_public IS DISTINCT FROM sub.should_be_public;
$$;

CREATE OR REPLACE FUNCTION public.fn_pe_sync_meta()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.embedding IS NOT NULL THEN
    NEW.embedding_hv := NEW.embedding::halfvec(1024);
  ELSE
    NEW.embedding_hv := NULL;
  END IF;

  SELECT p.taken_at
  INTO NEW.taken_at
  FROM public.photos p
  WHERE p.id = NEW.photo_id;

  SELECT EXISTS (
    SELECT 1
    FROM public.album_photos ap
    JOIN public.albums a ON a.id = ap.album_id
    WHERE ap.photo_id = NEW.photo_id
      AND a.is_public = true
  )
  INTO NEW.is_public;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_pe_sync_meta ON public.photo_embeddings;
CREATE TRIGGER trg_pe_sync_meta
BEFORE INSERT OR UPDATE OF embedding ON public.photo_embeddings
FOR EACH ROW
EXECUTE FUNCTION public.fn_pe_sync_meta();

CREATE OR REPLACE FUNCTION public.fn_photos_sync_pe_taken_at()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.taken_at IS DISTINCT FROM OLD.taken_at THEN
    UPDATE public.photo_embeddings
    SET taken_at = NEW.taken_at
    WHERE photo_id = NEW.id
      AND taken_at IS DISTINCT FROM NEW.taken_at;
  END IF;
  RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS trg_photos_sync_pe_taken_at ON public.photos;
CREATE TRIGGER trg_photos_sync_pe_taken_at
AFTER UPDATE OF taken_at ON public.photos
FOR EACH ROW
EXECUTE FUNCTION public.fn_photos_sync_pe_taken_at();

CREATE OR REPLACE FUNCTION public.fn_album_photos_sync_pe_public()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    IF EXISTS (SELECT 1 FROM public.albums WHERE id = NEW.album_id AND is_public = true) THEN
      UPDATE public.photo_embeddings
      SET is_public = true
      WHERE photo_id = NEW.photo_id
        AND is_public = false;
    END IF;
  ELSIF TG_OP = 'DELETE' THEN
    IF NOT EXISTS (SELECT 1 FROM public.albums WHERE id = OLD.album_id AND is_public = false) THEN
      PERFORM public.sync_photo_embedding_visibility(OLD.photo_id);
    END IF;
  ELSE
    IF OLD.photo_id IS DISTINCT FROM NEW.photo_id THEN
      PERFORM public.sync_photo_embedding_visibility(OLD.photo_id);
    END IF;
    PERFORM public.sync_photo_embedding_visibility(NEW.photo_id);
  END IF;
  RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS trg_album_photos_sync_pe_public ON public.album_photos;
CREATE TRIGGER trg_album_photos_sync_pe_public
AFTER INSERT OR UPDATE OF album_id, photo_id OR DELETE ON public.album_photos
FOR EACH ROW
EXECUTE FUNCTION public.fn_album_photos_sync_pe_public();

CREATE OR REPLACE FUNCTION public.fn_albums_sync_pe_public()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.is_public IS DISTINCT FROM OLD.is_public THEN
    UPDATE public.photo_embeddings pe
    SET is_public = src.should_be_public
    FROM (
      SELECT DISTINCT ap.photo_id,
             EXISTS (
               SELECT 1
               FROM public.album_photos ap2
               JOIN public.albums a2 ON a2.id = ap2.album_id
               WHERE ap2.photo_id = ap.photo_id
                 AND a2.is_public = true
             ) AS should_be_public
      FROM public.album_photos ap
      WHERE ap.album_id = NEW.id
    ) src
    WHERE pe.photo_id = src.photo_id
      AND pe.is_public IS DISTINCT FROM src.should_be_public;
  END IF;
  RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS trg_albums_sync_pe_public ON public.albums;
CREATE TRIGGER trg_albums_sync_pe_public
AFTER UPDATE OF is_public ON public.albums
FOR EACH ROW
EXECUTE FUNCTION public.fn_albums_sync_pe_public();

GRANT EXECUTE ON FUNCTION
  public.sync_photo_embedding_visibility(bigint),
  public.fn_pe_sync_meta(),
  public.fn_photos_sync_pe_taken_at(),
  public.fn_album_photos_sync_pe_public(),
  public.fn_albums_sync_pe_public()
TO photo_user;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.photo_embeddings TO photo_user;

COMMIT;

-- ============================================================================
-- Step 2: Backfill existing photo_embeddings rows
-- ============================================================================
UPDATE public.photo_embeddings pe
SET embedding_hv = pe.embedding::halfvec(1024),
    taken_at = (SELECT p.taken_at FROM public.photos p WHERE p.id = pe.photo_id),
    is_public = EXISTS (
      SELECT 1
      FROM public.album_photos ap
      JOIN public.albums a ON a.id = ap.album_id
      WHERE ap.photo_id = pe.photo_id
        AND a.is_public = true
    )
WHERE pe.embedding IS NOT NULL
  AND (
    pe.embedding_hv IS NULL
    OR pe.taken_at IS DISTINCT FROM (SELECT p.taken_at FROM public.photos p WHERE p.id = pe.photo_id)
    OR pe.is_public IS DISTINCT FROM EXISTS (
      SELECT 1
      FROM public.album_photos ap
      JOIN public.albums a ON a.id = ap.album_id
      WHERE ap.photo_id = pe.photo_id
        AND a.is_public = true
    )
  );

-- ============================================================================
-- Step 3: Build indexes online (CONCURRENTLY cannot run inside a transaction)
-- ============================================================================

-- Avoid parallel DSM allocations in Docker's default 64 MB /dev/shm when
-- building HNSW indexes; single-worker build uses normal backend RAM instead.
SET statement_timeout = 0;
SET max_parallel_maintenance_workers = 0;
SET maintenance_work_mem = '256MB';

-- Compact partial HNSW index for public photos (~75-85 MB, fits in NAS RAM).
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_pe_hnsw_public_hv
  ON public.photo_embeddings
  USING hnsw (embedding_hv halfvec_cosine_ops)
  WITH (m = 16, ef_construction = 128)
  WHERE is_public = true AND embedding_hv IS NOT NULL;

-- Compact full-library halfvec HNSW index (~240 MB vs 533 MB float32).
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_pe_hnsw_all_hv
  ON public.photo_embeddings
  USING hnsw (embedding_hv halfvec_cosine_ops)
  WITH (m = 16, ef_construction = 128)
  WHERE embedding_hv IS NOT NULL;

-- Composite B-Tree indexes for fast scope counts and time-range pre-filtering.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_pe_public_taken
  ON public.photo_embeddings (is_public, taken_at DESC, photo_id)
  WHERE taken_at IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_pe_taken_photo
  ON public.photo_embeddings (taken_at DESC, photo_id)
  WHERE taken_at IS NOT NULL;

-- Partial index for public albums.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_albums_public
  ON public.albums (id)
  WHERE is_public = true;

VACUUM ANALYZE public.photos, public.photo_embeddings, public.album_photos, public.albums;

-- ============================================================================
-- Step 4: Post-migration verification queries (run manually)
-- ============================================================================
-- SELECT count(*) AS missing_hv FROM public.photo_embeddings WHERE embedding_hv IS NULL;
-- SELECT count(*) AS pe_public FROM public.photo_embeddings WHERE is_public = true;
-- SELECT count(DISTINCT ap.photo_id) AS actual_public
--   FROM public.album_photos ap
--   JOIN public.albums a ON a.id = ap.album_id
--   JOIN public.photo_embeddings pe ON pe.photo_id = ap.photo_id
--  WHERE a.is_public = true;
-- SELECT indexname, pg_size_pretty(pg_relation_size(indexname::regclass)) AS size
--   FROM pg_indexes
--  WHERE schemaname = 'public' AND tablename = 'photo_embeddings'
--  ORDER BY indexname;
--
-- ============================================================================
-- Step 5: Optional cleanup AFTER new Go code is deployed & verified
-- ============================================================================
-- Dropping the legacy 533 MB float32 HNSW index reclaims ~533 MB of disk/cache:
--   DROP INDEX CONCURRENTLY IF EXISTS public.idx_embeddings_hnsw;
