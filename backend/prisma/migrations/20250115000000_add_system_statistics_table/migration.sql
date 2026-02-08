-- CreateTable
CREATE TABLE "system_statistics" (
    "id" TEXT NOT NULL,
    "unique_packages_count" INTEGER NOT NULL,
    "unique_security_count" INTEGER NOT NULL,
    "total_packages" INTEGER NOT NULL,
    "total_hosts" INTEGER NOT NULL,
    "hosts_needing_updates" INTEGER NOT NULL,
    "timestamp" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "system_statistics_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE INDEX "system_statistics_timestamp_idx" ON "system_statistics"("timestamp");

