-- Add can_manage_superusers column to role_permissions
ALTER TABLE "role_permissions" ADD COLUMN "can_manage_superusers" BOOLEAN NOT NULL DEFAULT false;

-- Set can_manage_superusers to true for superadmin role only
UPDATE "role_permissions" SET "can_manage_superusers" = true WHERE "role" = 'superadmin';
