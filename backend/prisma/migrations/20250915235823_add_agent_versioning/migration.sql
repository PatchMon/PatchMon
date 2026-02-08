-- AlterTable
ALTER TABLE "hosts" ADD COLUMN     "agent_version" TEXT;

-- CreateTable
CREATE TABLE "agent_versions" (
    "id" TEXT NOT NULL,
    "version" TEXT NOT NULL,
    "is_current" BOOLEAN NOT NULL DEFAULT false,
    "release_notes" TEXT,
    "download_url" TEXT,
    "min_server_version" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "agent_versions_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "agent_versions_version_key" ON "agent_versions"("version");
