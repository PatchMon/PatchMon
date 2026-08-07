ALTER TABLE users
    ADD COLUMN IF NOT EXISTS newsletter_subscribed BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS newsletter_subscribed_at TIMESTAMP(3);
