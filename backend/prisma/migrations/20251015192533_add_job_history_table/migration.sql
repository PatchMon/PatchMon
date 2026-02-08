-- CreateTable
CREATE TABLE "job_history" (
    "id" TEXT NOT NULL,
    "job_id" TEXT NOT NULL,
    "queue_name" TEXT NOT NULL,
    "job_name" TEXT NOT NULL,
    "host_id" TEXT,
    "api_id" TEXT,
    "status" TEXT NOT NULL,
    "attempt_number" INTEGER NOT NULL DEFAULT 1,
    "error_message" TEXT,
    "output" JSONB,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,
    "completed_at" TIMESTAMP(3),

    CONSTRAINT "job_history_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE INDEX "job_history_job_id_idx" ON "job_history"("job_id");

-- CreateIndex
CREATE INDEX "job_history_queue_name_idx" ON "job_history"("queue_name");

-- CreateIndex
CREATE INDEX "job_history_host_id_idx" ON "job_history"("host_id");

-- CreateIndex
CREATE INDEX "job_history_api_id_idx" ON "job_history"("api_id");

-- CreateIndex
CREATE INDEX "job_history_status_idx" ON "job_history"("status");

-- CreateIndex
CREATE INDEX "job_history_created_at_idx" ON "job_history"("created_at");

-- AddForeignKey
ALTER TABLE "job_history" ADD CONSTRAINT "job_history_host_id_fkey" FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE SET NULL ON UPDATE CASCADE;

