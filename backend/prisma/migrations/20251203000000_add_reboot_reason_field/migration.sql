-- Add reboot_reason field to hosts table
-- This field stores detailed technical information about why a reboot is required
-- Includes kernel versions, detection method, and other relevant details

ALTER TABLE "hosts" ADD COLUMN "reboot_reason" TEXT;

