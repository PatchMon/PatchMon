-- CreateTable
CREATE TABLE "compliance_profiles" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "type" TEXT NOT NULL,
    "os_family" TEXT,
    "version" TEXT,
    "description" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "compliance_profiles_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "compliance_scans" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "profile_id" TEXT NOT NULL,
    "started_at" TIMESTAMP(3) NOT NULL,
    "completed_at" TIMESTAMP(3),
    "status" TEXT NOT NULL,
    "total_rules" INTEGER NOT NULL DEFAULT 0,
    "passed" INTEGER NOT NULL DEFAULT 0,
    "failed" INTEGER NOT NULL DEFAULT 0,
    "warnings" INTEGER NOT NULL DEFAULT 0,
    "skipped" INTEGER NOT NULL DEFAULT 0,
    "score" DOUBLE PRECISION,
    "error_message" TEXT,
    "raw_output" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "compliance_scans_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "compliance_rules" (
    "id" TEXT NOT NULL,
    "profile_id" TEXT NOT NULL,
    "rule_ref" TEXT NOT NULL,
    "title" TEXT NOT NULL,
    "description" TEXT,
    "rationale" TEXT,
    "severity" TEXT,
    "section" TEXT,
    "remediation" TEXT,

    CONSTRAINT "compliance_rules_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "compliance_results" (
    "id" TEXT NOT NULL,
    "scan_id" TEXT NOT NULL,
    "rule_id" TEXT NOT NULL,
    "status" TEXT NOT NULL,
    "finding" TEXT,
    "actual" TEXT,
    "expected" TEXT,
    "remediation" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "compliance_results_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "compliance_profiles_name_key" ON "compliance_profiles"("name");

-- CreateIndex
CREATE INDEX "compliance_profiles_type_idx" ON "compliance_profiles"("type");

-- CreateIndex
CREATE INDEX "compliance_profiles_os_family_idx" ON "compliance_profiles"("os_family");

-- CreateIndex
CREATE INDEX "compliance_scans_host_id_idx" ON "compliance_scans"("host_id");

-- CreateIndex
CREATE INDEX "compliance_scans_profile_id_idx" ON "compliance_scans"("profile_id");

-- CreateIndex
CREATE INDEX "compliance_scans_status_idx" ON "compliance_scans"("status");

-- CreateIndex
CREATE INDEX "compliance_scans_started_at_idx" ON "compliance_scans"("started_at");

-- CreateIndex
CREATE INDEX "compliance_scans_completed_at_idx" ON "compliance_scans"("completed_at");

-- CreateIndex
CREATE INDEX "compliance_scans_host_id_started_at_idx" ON "compliance_scans"("host_id", "started_at");

-- CreateIndex
CREATE INDEX "compliance_scans_host_id_profile_id_idx" ON "compliance_scans"("host_id", "profile_id");

-- CreateIndex
CREATE INDEX "compliance_rules_profile_id_idx" ON "compliance_rules"("profile_id");

-- CreateIndex
CREATE INDEX "compliance_rules_severity_idx" ON "compliance_rules"("severity");

-- CreateIndex
CREATE INDEX "compliance_rules_section_idx" ON "compliance_rules"("section");

-- CreateIndex
CREATE UNIQUE INDEX "compliance_rules_profile_id_rule_ref_key" ON "compliance_rules"("profile_id", "rule_ref");

-- CreateIndex
CREATE INDEX "compliance_results_scan_id_idx" ON "compliance_results"("scan_id");

-- CreateIndex
CREATE INDEX "compliance_results_rule_id_idx" ON "compliance_results"("rule_id");

-- CreateIndex
CREATE INDEX "compliance_results_status_idx" ON "compliance_results"("status");

-- CreateIndex
CREATE INDEX "compliance_results_scan_id_status_idx" ON "compliance_results"("scan_id", "status");

-- CreateIndex
CREATE UNIQUE INDEX "compliance_results_scan_id_rule_id_key" ON "compliance_results"("scan_id", "rule_id");

-- AddForeignKey
ALTER TABLE "compliance_scans" ADD CONSTRAINT "compliance_scans_host_id_fkey" FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "compliance_scans" ADD CONSTRAINT "compliance_scans_profile_id_fkey" FOREIGN KEY ("profile_id") REFERENCES "compliance_profiles"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "compliance_rules" ADD CONSTRAINT "compliance_rules_profile_id_fkey" FOREIGN KEY ("profile_id") REFERENCES "compliance_profiles"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "compliance_results" ADD CONSTRAINT "compliance_results_scan_id_fkey" FOREIGN KEY ("scan_id") REFERENCES "compliance_scans"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "compliance_results" ADD CONSTRAINT "compliance_results_rule_id_fkey" FOREIGN KEY ("rule_id") REFERENCES "compliance_rules"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- Add compliance_enabled column to hosts table
ALTER TABLE "hosts" ADD COLUMN IF NOT EXISTS "compliance_enabled" BOOLEAN DEFAULT false;
