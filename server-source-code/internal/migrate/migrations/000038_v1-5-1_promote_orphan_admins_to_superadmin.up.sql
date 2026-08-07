-- One-time upgrade safety net: promote all existing `admin` users to `superadmin`
-- IF AND ONLY IF no `superadmin` user currently exists.
--
-- Context: the `superadmin` role was introduced in v1.4.0. Installations that
-- predate it only have `admin` users, and those admins cannot self-promote
-- via the UI/API (canAssignRole in handler/users.go blocks admin -> superadmin).
-- Without this migration, upgraded instances would be stuck with no one able
-- to manage other superadmins, billing, or assign the admin role to new users.
--
-- Safety:
--   * Runs inside the migration transaction.
--   * Idempotent: the NOT EXISTS guard makes re-running a no-op.
--   * Never touches users on installs that already have a superadmin
--     (fresh installs, or instances where a superadmin was created manually).
--   * Promotes ALL admin users (there may legitimately be more than one).

DO $$
DECLARE
    promoted_count INTEGER := 0;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM users WHERE role = 'superadmin') THEN
        UPDATE users
           SET role       = 'superadmin',
               updated_at = CURRENT_TIMESTAMP
         WHERE role = 'admin';

        GET DIAGNOSTICS promoted_count = ROW_COUNT;

        IF promoted_count > 0 THEN
            RAISE NOTICE
              '[migrate 000038] Promoted % admin user(s) to superadmin (no superadmin existed).',
              promoted_count;
        END IF;
    END IF;
END $$;
