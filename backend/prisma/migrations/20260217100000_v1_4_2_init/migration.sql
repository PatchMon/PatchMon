-- v1.4.2 init: merge of (1) ram/swap float, (2) compliance scanner status on hosts,
-- (3) dashboard_layout table, (4) dashboard_preferences col_span.
-- Data is preserved; new columns/tables have safe defaults.

-- 1) Change ram_installed and swap_size from INTEGER to DOUBLE PRECISION (Float)
-- to preserve decimal precision from the agent's reported values.
-- Existing integer data is safely promoted (e.g. 3 → 3.0) with no data loss.
ALTER TABLE "hosts" ALTER COLUMN "ram_installed" SET DATA TYPE DOUBLE PRECISION USING "ram_installed"::DOUBLE PRECISION;
ALTER TABLE "hosts" ALTER COLUMN "swap_size" SET DATA TYPE DOUBLE PRECISION USING "swap_size"::DOUBLE PRECISION;

-- 2) Persist last known compliance scanner status on host so UI can show accurate state
-- when Redis cache is empty. New columns are nullable; no existing data modified.
ALTER TABLE "hosts" ADD COLUMN IF NOT EXISTS "compliance_scanner_status" JSONB;
ALTER TABLE "hosts" ADD COLUMN IF NOT EXISTS "compliance_scanner_updated_at" TIMESTAMP(3);

-- 3) dashboard_layout: per-user row column counts (stats row, charts row).
CREATE TABLE "dashboard_layout" (
    "user_id" TEXT NOT NULL,
    "stats_columns" INTEGER NOT NULL DEFAULT 5,
    "charts_columns" INTEGER NOT NULL DEFAULT 3,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "dashboard_layout_pkey" PRIMARY KEY ("user_id")
);
ALTER TABLE "dashboard_layout" ADD CONSTRAINT "dashboard_layout_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- 4) Add col_span to dashboard_preferences (how many columns the card spans in its row).
-- Default 1 preserves existing behaviour; existing rows get 1.
ALTER TABLE "dashboard_preferences" ADD COLUMN "col_span" INTEGER NOT NULL DEFAULT 1;
