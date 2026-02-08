-- CreateTable
CREATE TABLE "host_group_memberships" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "host_group_id" TEXT NOT NULL,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "host_group_memberships_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "host_group_memberships_host_id_host_group_id_key" ON "host_group_memberships"("host_id", "host_group_id");

-- CreateIndex
CREATE INDEX "host_group_memberships_host_id_idx" ON "host_group_memberships"("host_id");

-- CreateIndex
CREATE INDEX "host_group_memberships_host_group_id_idx" ON "host_group_memberships"("host_group_id");

-- Migrate existing data from hosts.host_group_id to host_group_memberships
INSERT INTO "host_group_memberships" ("id", "host_id", "host_group_id", "created_at")
SELECT 
    gen_random_uuid()::text as "id",
    "id" as "host_id", 
    "host_group_id" as "host_group_id",
    CURRENT_TIMESTAMP as "created_at"
FROM "hosts" 
WHERE "host_group_id" IS NOT NULL;

-- AddForeignKey
ALTER TABLE "host_group_memberships" ADD CONSTRAINT "host_group_memberships_host_id_fkey" FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "host_group_memberships" ADD CONSTRAINT "host_group_memberships_host_group_id_fkey" FOREIGN KEY ("host_group_id") REFERENCES "host_groups"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- DropForeignKey
ALTER TABLE "hosts" DROP CONSTRAINT IF EXISTS "hosts_host_group_id_fkey";

-- DropIndex
DROP INDEX IF EXISTS "hosts_host_group_id_idx";

-- AlterTable
ALTER TABLE "hosts" DROP COLUMN "host_group_id";
