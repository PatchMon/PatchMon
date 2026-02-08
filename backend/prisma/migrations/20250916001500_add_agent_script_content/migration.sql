-- AlterTable
ALTER TABLE "agent_versions" ADD COLUMN     "is_default" BOOLEAN NOT NULL DEFAULT false,
ADD COLUMN     "script_content" TEXT;
