-- Add indexes to host_packages table for performance optimization
-- These indexes will dramatically speed up queries filtering by host_id, package_id, needs_update, and is_security_update

-- Index for queries filtering by host_id (very common - used when viewing packages for a specific host)
CREATE INDEX IF NOT EXISTS "host_packages_host_id_idx" ON "host_packages"("host_id");

-- Index for queries filtering by package_id (used when finding hosts for a specific package)
CREATE INDEX IF NOT EXISTS "host_packages_package_id_idx" ON "host_packages"("package_id");

-- Index for queries filtering by needs_update (used when finding outdated packages)
CREATE INDEX IF NOT EXISTS "host_packages_needs_update_idx" ON "host_packages"("needs_update");

-- Index for queries filtering by is_security_update (used when finding security updates)
CREATE INDEX IF NOT EXISTS "host_packages_is_security_update_idx" ON "host_packages"("is_security_update");

-- Composite index for the most common query pattern: host_id + needs_update
-- This is optimal for "show me outdated packages for this host"
CREATE INDEX IF NOT EXISTS "host_packages_host_id_needs_update_idx" ON "host_packages"("host_id", "needs_update");

-- Composite index for host_id + needs_update + is_security_update
-- This is optimal for "show me security updates for this host"
CREATE INDEX IF NOT EXISTS "host_packages_host_id_needs_update_security_idx" ON "host_packages"("host_id", "needs_update", "is_security_update");

-- Index for queries filtering by package_id + needs_update
-- This is optimal for "show me hosts where this package needs updates"
CREATE INDEX IF NOT EXISTS "host_packages_package_id_needs_update_idx" ON "host_packages"("package_id", "needs_update");

-- Index on last_checked for cleanup/maintenance queries
CREATE INDEX IF NOT EXISTS "host_packages_last_checked_idx" ON "host_packages"("last_checked");

