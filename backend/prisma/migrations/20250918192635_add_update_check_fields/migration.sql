-- AlterTable
ALTER TABLE "settings" ADD COLUMN     "last_update_check" TIMESTAMP(3),
ADD COLUMN     "latest_version" TEXT,
ADD COLUMN     "update_available" BOOLEAN NOT NULL DEFAULT false;
