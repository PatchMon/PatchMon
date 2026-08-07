ALTER TABLE role_permissions
    DROP COLUMN IF EXISTS can_manage_patching,
    DROP COLUMN IF EXISTS can_manage_compliance,
    DROP COLUMN IF EXISTS can_manage_docker,
    DROP COLUMN IF EXISTS can_manage_alerts,
    DROP COLUMN IF EXISTS can_manage_automation,
    DROP COLUMN IF EXISTS can_use_remote_access;
