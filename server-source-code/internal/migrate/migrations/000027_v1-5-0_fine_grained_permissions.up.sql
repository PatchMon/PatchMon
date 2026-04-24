-- Fine-grained RBAC: split can_manage_hosts into dedicated permissions

ALTER TABLE role_permissions
    ADD COLUMN IF NOT EXISTS can_manage_patching BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS can_manage_compliance BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS can_manage_docker BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS can_manage_alerts BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS can_manage_automation BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS can_use_remote_access BOOLEAN NOT NULL DEFAULT false;

-- superadmin: full access
UPDATE role_permissions SET
    can_manage_patching = true,
    can_manage_compliance = true,
    can_manage_docker = true,
    can_manage_alerts = true,
    can_manage_automation = true,
    can_use_remote_access = true
WHERE role = 'superadmin';

-- admin: full access
UPDATE role_permissions SET
    can_manage_patching = true,
    can_manage_compliance = true,
    can_manage_docker = true,
    can_manage_alerts = true,
    can_manage_automation = true,
    can_use_remote_access = true
WHERE role = 'admin';

-- host_manager: operational access (matches previous can_manage_hosts = true behavior)
UPDATE role_permissions SET
    can_manage_patching = true,
    can_manage_compliance = true,
    can_manage_docker = true,
    can_manage_alerts = true,
    can_manage_automation = true,
    can_use_remote_access = true
WHERE role = 'host_manager';

-- readonly: no write access
-- user: no write access
-- (both default to false, no UPDATE needed)
