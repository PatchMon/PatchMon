-- Migration: v1.4.1 Add expected_platform to hosts (for OS-specific install command)
-- Safe: additive only, nullable column, no data loss

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public'
        AND table_name = 'hosts'
        AND column_name = 'expected_platform'
    ) THEN
        ALTER TABLE "hosts" ADD COLUMN "expected_platform" TEXT;
    END IF;
END $$;
