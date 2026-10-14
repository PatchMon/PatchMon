CREATE OR REPLACE FUNCTION pm_tmp_pct_decode(s TEXT) RETURNS TEXT LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE
    buf BYTEA := ''::BYTEA;
    i INT := 1;
    n INT := length(s);
    c TEXT;
BEGIN
    WHILE i <= n LOOP
        c := substr(s, i, 1);
        IF c = '%' AND substr(s, i + 1, 2) ~ '^[0-9A-Fa-f]{2}$' THEN
            buf := buf || decode(substr(s, i + 1, 2), 'hex');
            i := i + 3;
        ELSE
            buf := buf || convert_to(c, 'UTF8');
            i := i + 1;
        END IF;
    END LOOP;
    RETURN convert_from(buf, 'UTF8');
EXCEPTION WHEN OTHERS THEN
    RETURN s;
END
$$;

-- A colliding row with different permissions is left alone so no user changes privileges.
DO $$
DECLARE
    r RECORD;
    decoded TEXT;
    existing role_permissions%ROWTYPE;
BEGIN
    FOR r IN SELECT * FROM role_permissions WHERE role ~ '%[0-9A-Fa-f]{2}' ORDER BY role LOOP
        decoded := pm_tmp_pct_decode(r.role);
        CONTINUE WHEN decoded = r.role OR btrim(decoded) = '';
        IF lower(btrim(decoded)) IN ('superadmin', 'admin', 'host_manager', 'readonly', 'user') THEN
            RAISE WARNING 'role % not renamed: it decodes to the built-in role %', r.role, decoded;
            CONTINUE;
        END IF;

        SELECT * INTO existing FROM role_permissions WHERE role = decoded;
        IF FOUND THEN
            IF (to_jsonb(existing) - 'id' - 'role' - 'created_at' - 'updated_at')
               IS DISTINCT FROM (to_jsonb(r) - 'id' - 'role' - 'created_at' - 'updated_at') THEN
                RAISE WARNING 'role % not renamed to %: a role with that name already exists with different permissions', r.role, decoded;
                CONTINUE;
            END IF;
            DELETE FROM role_permissions WHERE id = r.id;
        ELSE
            UPDATE role_permissions SET role = decoded, updated_at = CURRENT_TIMESTAMP WHERE id = r.id;
        END IF;

        -- Their access tokens carry the old role name; dropping the sessions makes the next request 401.
        DELETE FROM user_sessions WHERE user_id IN (SELECT id FROM users WHERE role = r.role);
        UPDATE users SET role = decoded, updated_at = CURRENT_TIMESTAMP WHERE role = r.role;
        UPDATE settings SET default_user_role = decoded WHERE default_user_role = r.role;
        UPDATE settings SET oidc_default_role = decoded WHERE oidc_default_role = r.role;
    END LOOP;
END
$$;

DROP FUNCTION pm_tmp_pct_decode(TEXT);
