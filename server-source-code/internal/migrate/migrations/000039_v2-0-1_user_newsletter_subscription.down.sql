ALTER TABLE users
    DROP COLUMN IF EXISTS newsletter_subscribed_at,
    DROP COLUMN IF EXISTS newsletter_subscribed;
