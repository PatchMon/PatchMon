-- Adds can_manage_billing permission for the PatchMon-native Billing page.
-- This page is double-gated: ADMIN_MODE=on AND the user's role has
-- can_manage_billing = true. Defaults to TRUE for superadmin/admin only.

ALTER TABLE role_permissions
    ADD COLUMN IF NOT EXISTS can_manage_billing BOOLEAN NOT NULL DEFAULT false;

UPDATE role_permissions SET can_manage_billing = true
    WHERE role IN ('superadmin', 'admin');
