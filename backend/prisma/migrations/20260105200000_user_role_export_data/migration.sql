-- Update user role to have can_export_data permission
-- This differentiates "user" from "readonly" role
-- Hierarchy: readonly < user < host_manager < admin < superadmin

UPDATE role_permissions
SET can_export_data = true, updated_at = NOW()
WHERE role = 'user' AND can_export_data = false;
