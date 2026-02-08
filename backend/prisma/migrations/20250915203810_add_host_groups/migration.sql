-- AlterTable
ALTER TABLE "hosts" ADD COLUMN     "host_group_id" TEXT;

-- CreateTable
CREATE TABLE "host_groups" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "description" TEXT,
    "color" TEXT DEFAULT '#3B82F6',
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "host_groups_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "host_groups_name_key" ON "host_groups"("name");

-- AddForeignKey
ALTER TABLE "hosts" ADD CONSTRAINT "hosts_host_group_id_fkey" FOREIGN KEY ("host_group_id") REFERENCES "host_groups"("id") ON DELETE SET NULL ON UPDATE CASCADE;
