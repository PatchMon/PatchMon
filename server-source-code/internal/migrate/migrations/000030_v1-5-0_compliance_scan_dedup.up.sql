-- Deduplicate completed compliance_scans, then add partial unique index.
-- Idempotent: runs once per DB (golang-migrate). Not re-executed on every deploy.
-- Strategy: if completed_at+created_at exist, keep newest by time; else keep highest id
-- for completed rows. Import stacks missing timestamp columns still get a safe dedupe + index.
DO $body$
DECLARE
  has_id boolean;
  has_host boolean;
  has_profile boolean;
  has_status boolean;
  has_completed_at boolean;
  has_created_at boolean;
  dupes_without_id boolean;
BEGIN
  IF to_regclass('public.compliance_scans') IS NULL THEN
    RAISE WARNING '000030: public.compliance_scans not found, skipping';
    RETURN;
  END IF;

  SELECT EXISTS (SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'compliance_scans' AND column_name = 'id') INTO has_id;
  SELECT EXISTS (SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'compliance_scans' AND column_name = 'host_id') INTO has_host;
  SELECT EXISTS (SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'compliance_scans' AND column_name = 'profile_id') INTO has_profile;
  SELECT EXISTS (SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'compliance_scans' AND column_name = 'status') INTO has_status;
  SELECT EXISTS (SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'compliance_scans' AND column_name = 'completed_at') INTO has_completed_at;
  SELECT EXISTS (SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'compliance_scans' AND column_name = 'created_at') INTO has_created_at;

  IF NOT (has_host AND has_profile AND has_status) THEN
    RAISE WARNING '000030: compliance_scans needs host_id, profile_id, and status, skipping';
    RETURN;
  END IF;

  IF has_id THEN
    IF has_completed_at AND has_created_at THEN
      DELETE FROM public.compliance_scans
      WHERE id IN (
        SELECT id FROM (
          SELECT id,
                 ROW_NUMBER() OVER (
                   PARTITION BY host_id, profile_id
                   ORDER BY completed_at DESC NULLS LAST, created_at DESC
                 ) AS rn
          FROM public.compliance_scans
          WHERE status = 'completed'
        ) ranked
        WHERE rn > 1
      );
    ELSE
      DELETE FROM public.compliance_scans
      WHERE id IN (
        SELECT id FROM (
          SELECT id,
                 ROW_NUMBER() OVER (
                   PARTITION BY host_id, profile_id
                   ORDER BY id DESC
                 ) AS rn
          FROM public.compliance_scans
          WHERE status = 'completed'
        ) ranked
        WHERE rn > 1
      );
    END IF;
  ELSE
    SELECT EXISTS (
      SELECT 1
      FROM public.compliance_scans
      WHERE status = 'completed'
      GROUP BY host_id, profile_id
      HAVING COUNT(*) > 1
    ) INTO dupes_without_id;
    IF dupes_without_id THEN
      RAISE EXCEPTION
        '000030: compliance_scans has duplicate completed (host_id, profile_id) rows and no id column; align the table to PatchMon (see 000001) or remove duplicate rows, then re-run migration';
    END IF;
  END IF;

  EXECUTE $idx$
    CREATE UNIQUE INDEX IF NOT EXISTS idx_compliance_scans_host_profile_completed
    ON public.compliance_scans (host_id, profile_id)
    WHERE status = 'completed'
  $idx$;
END
$body$;
