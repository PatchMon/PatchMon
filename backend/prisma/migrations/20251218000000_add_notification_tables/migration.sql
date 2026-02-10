-- CreateTable notification_channels
CREATE TABLE "notification_channels" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "channel_type" TEXT NOT NULL DEFAULT 'gotify',
    "server_url" TEXT NOT NULL,
    "token" TEXT NOT NULL,
    "priority" INTEGER NOT NULL DEFAULT 5,
    "status" TEXT NOT NULL DEFAULT 'connected',
    "last_tested_at" TIMESTAMP(3),
    "last_error" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,
    "created_by_user_id" TEXT,

    CONSTRAINT "notification_channels_pkey" PRIMARY KEY ("id")
);

-- CreateTable notification_rules
CREATE TABLE "notification_rules" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "description" TEXT,
    "event_type" TEXT NOT NULL,
    "enabled" BOOLEAN NOT NULL DEFAULT true,
    "priority" INTEGER NOT NULL DEFAULT 5,
    "message_title" TEXT,
    "message_template" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,
    "created_by_user_id" TEXT,

    CONSTRAINT "notification_rules_pkey" PRIMARY KEY ("id")
);

-- CreateTable notification_rule_channels
CREATE TABLE "notification_rule_channels" (
    "id" TEXT NOT NULL,
    "rule_id" TEXT NOT NULL,
    "channel_id" TEXT NOT NULL,

    CONSTRAINT "notification_rule_channels_pkey" PRIMARY KEY ("id")
);

-- CreateTable notification_rule_filters
CREATE TABLE "notification_rule_filters" (
    "id" TEXT NOT NULL,
    "rule_id" TEXT NOT NULL,
    "filter_type" TEXT NOT NULL,
    "filter_value" TEXT NOT NULL,

    CONSTRAINT "notification_rule_filters_pkey" PRIMARY KEY ("id")
);

-- CreateTable notification_history
CREATE TABLE "notification_history" (
    "id" TEXT NOT NULL,
    "rule_id" TEXT,
    "channel_id" TEXT NOT NULL,
    "event_type" TEXT NOT NULL,
    "status" TEXT NOT NULL,
    "message_title" TEXT,
    "message_content" TEXT,
    "error_message" TEXT,
    "sent_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "notification_history_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE INDEX "notification_channels_status_idx" ON "notification_channels"("status");

-- CreateIndex
CREATE INDEX "notification_channels_created_by_user_id_idx" ON "notification_channels"("created_by_user_id");

-- CreateIndex
CREATE INDEX "notification_rules_event_type_idx" ON "notification_rules"("event_type");

-- CreateIndex
CREATE INDEX "notification_rules_enabled_idx" ON "notification_rules"("enabled");

-- CreateIndex
CREATE INDEX "notification_rules_created_by_user_id_idx" ON "notification_rules"("created_by_user_id");

-- CreateIndex
CREATE UNIQUE INDEX "notification_rule_channels_rule_id_channel_id_key" ON "notification_rule_channels"("rule_id", "channel_id");

-- CreateIndex
CREATE INDEX "notification_rule_channels_rule_id_idx" ON "notification_rule_channels"("rule_id");

-- CreateIndex
CREATE INDEX "notification_rule_channels_channel_id_idx" ON "notification_rule_channels"("channel_id");

-- CreateIndex
CREATE INDEX "notification_rule_filters_rule_id_idx" ON "notification_rule_filters"("rule_id");

-- CreateIndex
CREATE INDEX "notification_history_sent_at_idx" ON "notification_history"("sent_at");

-- CreateIndex
CREATE INDEX "notification_history_channel_id_idx" ON "notification_history"("channel_id");

-- CreateIndex
CREATE INDEX "notification_history_status_idx" ON "notification_history"("status");

-- CreateIndex
CREATE INDEX "notification_history_event_type_idx" ON "notification_history"("event_type");

-- AddForeignKey
ALTER TABLE "notification_channels" ADD CONSTRAINT "notification_channels_created_by_user_id_fkey" FOREIGN KEY ("created_by_user_id") REFERENCES "users"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "notification_rules" ADD CONSTRAINT "notification_rules_created_by_user_id_fkey" FOREIGN KEY ("created_by_user_id") REFERENCES "users"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "notification_rule_channels" ADD CONSTRAINT "notification_rule_channels_rule_id_fkey" FOREIGN KEY ("rule_id") REFERENCES "notification_rules"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "notification_rule_channels" ADD CONSTRAINT "notification_rule_channels_channel_id_fkey" FOREIGN KEY ("channel_id") REFERENCES "notification_channels"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "notification_rule_filters" ADD CONSTRAINT "notification_rule_filters_rule_id_fkey" FOREIGN KEY ("rule_id") REFERENCES "notification_rules"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "notification_history" ADD CONSTRAINT "notification_history_rule_id_fkey" FOREIGN KEY ("rule_id") REFERENCES "notification_rules"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "notification_history" ADD CONSTRAINT "notification_history_channel_id_fkey" FOREIGN KEY ("channel_id") REFERENCES "notification_channels"("id") ON DELETE CASCADE ON UPDATE CASCADE;
