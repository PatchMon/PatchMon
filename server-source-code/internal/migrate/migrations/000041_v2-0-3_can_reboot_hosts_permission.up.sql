-- Adds can_reboot_hosts permission for the remote reboot feature.
-- Intentionally more restrictive than can_manage_hosts: rebooting is a
-- destructive action, so it defaults to TRUE for superadmin/admin only.

ALTER TABLE role_permissions
    ADD COLUMN IF NOT EXISTS can_reboot_hosts BOOLEAN NOT NULL DEFAULT false;

UPDATE role_permissions SET can_reboot_hosts = true
    WHERE role IN ('superadmin', 'admin');
