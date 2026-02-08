-- Create compliance_profiles table if it doesn't exist (must be created first as other tables depend on it)
CREATE TABLE IF NOT EXISTS "compliance_profiles" (
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

-- Create compliance_scans table if it doesn't exist
CREATE TABLE IF NOT EXISTS "compliance_scans" (
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
    "not_applicable" INTEGER NOT NULL DEFAULT 0,
    "score" DOUBLE PRECISION,
    "error_message" TEXT,
    "raw_output" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "compliance_scans_pkey" PRIMARY KEY ("id")
);

-- Create compliance_rules table if it doesn't exist
CREATE TABLE IF NOT EXISTS "compliance_rules" (
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

-- Create compliance_results table if it doesn't exist
CREATE TABLE IF NOT EXISTS "compliance_results" (
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

-- Create indexes if they don't exist (using DO block for conditional creation)
DO $$
BEGIN
    -- compliance_profiles indexes
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_profiles_name_key') THEN
        CREATE UNIQUE INDEX "compliance_profiles_name_key" ON "compliance_profiles"("name");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_profiles_type_idx') THEN
        CREATE INDEX "compliance_profiles_type_idx" ON "compliance_profiles"("type");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_profiles_os_family_idx') THEN
        CREATE INDEX "compliance_profiles_os_family_idx" ON "compliance_profiles"("os_family");
    END IF;
    
    -- compliance_scans indexes
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_scans_host_id_idx') THEN
        CREATE INDEX "compliance_scans_host_id_idx" ON "compliance_scans"("host_id");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_scans_profile_id_idx') THEN
        CREATE INDEX "compliance_scans_profile_id_idx" ON "compliance_scans"("profile_id");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_scans_status_idx') THEN
        CREATE INDEX "compliance_scans_status_idx" ON "compliance_scans"("status");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_scans_started_at_idx') THEN
        CREATE INDEX "compliance_scans_started_at_idx" ON "compliance_scans"("started_at");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_scans_completed_at_idx') THEN
        CREATE INDEX "compliance_scans_completed_at_idx" ON "compliance_scans"("completed_at");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_scans_host_id_started_at_idx') THEN
        CREATE INDEX "compliance_scans_host_id_started_at_idx" ON "compliance_scans"("host_id", "started_at");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_scans_host_id_profile_id_idx') THEN
        CREATE INDEX "compliance_scans_host_id_profile_id_idx" ON "compliance_scans"("host_id", "profile_id");
    END IF;
    
    -- compliance_rules indexes
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_rules_profile_id_idx') THEN
        CREATE INDEX "compliance_rules_profile_id_idx" ON "compliance_rules"("profile_id");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_rules_severity_idx') THEN
        CREATE INDEX "compliance_rules_severity_idx" ON "compliance_rules"("severity");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_rules_section_idx') THEN
        CREATE INDEX "compliance_rules_section_idx" ON "compliance_rules"("section");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_rules_profile_id_rule_ref_key') THEN
        CREATE UNIQUE INDEX "compliance_rules_profile_id_rule_ref_key" ON "compliance_rules"("profile_id", "rule_ref");
    END IF;
    
    -- compliance_results indexes
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_results_scan_id_idx') THEN
        CREATE INDEX "compliance_results_scan_id_idx" ON "compliance_results"("scan_id");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_results_rule_id_idx') THEN
        CREATE INDEX "compliance_results_rule_id_idx" ON "compliance_results"("rule_id");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_results_status_idx') THEN
        CREATE INDEX "compliance_results_status_idx" ON "compliance_results"("status");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_results_scan_id_status_idx') THEN
        CREATE INDEX "compliance_results_scan_id_status_idx" ON "compliance_results"("scan_id", "status");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_results_scan_id_rule_id_key') THEN
        CREATE UNIQUE INDEX "compliance_results_scan_id_rule_id_key" ON "compliance_results"("scan_id", "rule_id");
    END IF;
END $$;

-- Add foreign key constraints if they don't exist
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'compliance_scans_host_id_fkey'
    ) THEN
        ALTER TABLE "compliance_scans" 
        ADD CONSTRAINT "compliance_scans_host_id_fkey" 
        FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE CASCADE ON UPDATE CASCADE;
    END IF;
    
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'compliance_scans_profile_id_fkey'
    ) THEN
        ALTER TABLE "compliance_scans" 
        ADD CONSTRAINT "compliance_scans_profile_id_fkey" 
        FOREIGN KEY ("profile_id") REFERENCES "compliance_profiles"("id") ON DELETE CASCADE ON UPDATE CASCADE;
    END IF;
    
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'compliance_rules_profile_id_fkey'
    ) THEN
        ALTER TABLE "compliance_rules" 
        ADD CONSTRAINT "compliance_rules_profile_id_fkey" 
        FOREIGN KEY ("profile_id") REFERENCES "compliance_profiles"("id") ON DELETE CASCADE ON UPDATE CASCADE;
    END IF;
    
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'compliance_results_scan_id_fkey'
    ) THEN
        ALTER TABLE "compliance_results" 
        ADD CONSTRAINT "compliance_results_scan_id_fkey" 
        FOREIGN KEY ("scan_id") REFERENCES "compliance_scans"("id") ON DELETE CASCADE ON UPDATE CASCADE;
    END IF;
    
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'compliance_results_rule_id_fkey'
    ) THEN
        ALTER TABLE "compliance_results" 
        ADD CONSTRAINT "compliance_results_rule_id_fkey" 
        FOREIGN KEY ("rule_id") REFERENCES "compliance_rules"("id") ON DELETE CASCADE ON UPDATE CASCADE;
    END IF;
END $$;

