-- Rename hostname column to friendly_name in hosts table
ALTER TABLE "hosts" RENAME COLUMN "hostname" TO "friendly_name";
