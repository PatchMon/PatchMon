-- Initialize default dashboard preferences for all existing users
-- This migration ensures that all users have proper role-based dashboard preferences

-- Function to create default dashboard preferences for a user
CREATE OR REPLACE FUNCTION init_user_dashboard_preferences(user_id TEXT, user_role TEXT)
RETURNS VOID AS $$
DECLARE
    pref_record RECORD;
BEGIN
    -- Delete any existing preferences for this user
    DELETE FROM dashboard_preferences WHERE dashboard_preferences.user_id = init_user_dashboard_preferences.user_id;
    
    -- Insert role-based preferences
    IF user_role = 'admin' THEN
        -- Admin gets full access to all cards (iby's preferred layout)
        INSERT INTO dashboard_preferences (id, user_id, card_id, enabled, "order", created_at, updated_at)
        VALUES 
            (gen_random_uuid(), user_id, 'totalHosts', true, 0, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'hostsNeedingUpdates', true, 1, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'totalOutdatedPackages', true, 2, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'securityUpdates', true, 3, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'totalHostGroups', true, 4, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'upToDateHosts', true, 5, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'totalRepos', true, 6, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'totalUsers', true, 7, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'osDistribution', true, 8, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'osDistributionBar', true, 9, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'recentCollection', true, 10, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'updateStatus', true, 11, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'packagePriority', true, 12, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'recentUsers', true, 13, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'quickStats', true, 14, NOW(), NOW());
    ELSE
        -- Regular users get comprehensive layout but without user management cards
        INSERT INTO dashboard_preferences (id, user_id, card_id, enabled, "order", created_at, updated_at)
        VALUES 
            (gen_random_uuid(), user_id, 'totalHosts', true, 0, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'hostsNeedingUpdates', true, 1, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'totalOutdatedPackages', true, 2, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'securityUpdates', true, 3, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'totalHostGroups', true, 4, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'upToDateHosts', true, 5, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'totalRepos', true, 6, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'osDistribution', true, 7, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'osDistributionBar', true, 8, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'recentCollection', true, 9, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'updateStatus', true, 10, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'packagePriority', true, 11, NOW(), NOW()),
            (gen_random_uuid(), user_id, 'quickStats', true, 12, NOW(), NOW());
    END IF;
END;
$$ LANGUAGE plpgsql;

-- Apply default preferences to all existing users
DO $$
DECLARE
    user_record RECORD;
BEGIN
    FOR user_record IN SELECT id, role FROM users LOOP
        PERFORM init_user_dashboard_preferences(user_record.id, user_record.role);
    END LOOP;
END $$;

-- Drop the temporary function
DROP FUNCTION init_user_dashboard_preferences(TEXT, TEXT);
