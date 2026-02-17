-- Change ram_installed and swap_size from INTEGER to DOUBLE PRECISION (Float)
-- to preserve decimal precision from the agent's reported values.
-- Previously, a float like 3.84 was truncated to 3 when stored as INT.
-- Existing integer data is safely promoted (e.g. 3 → 3.0) with no data loss.

ALTER TABLE "hosts" ALTER COLUMN "ram_installed" SET DATA TYPE DOUBLE PRECISION USING "ram_installed"::DOUBLE PRECISION;
ALTER TABLE "hosts" ALTER COLUMN "swap_size" SET DATA TYPE DOUBLE PRECISION USING "swap_size"::DOUBLE PRECISION;
