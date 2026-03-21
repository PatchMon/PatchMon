-- Remove default routes to internal destination.
DELETE FROM notification_routes WHERE destination_id = 'internal-alerts';
-- Remove the built-in internal destination.
DELETE FROM notification_destinations WHERE id = 'internal-alerts';
