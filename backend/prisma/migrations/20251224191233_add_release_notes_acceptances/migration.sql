-- CreateTable
CREATE TABLE "release_notes_acceptances" (
    "id" TEXT NOT NULL,
    "user_id" TEXT NOT NULL,
    "version" TEXT NOT NULL,
    "accepted_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "release_notes_acceptances_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "release_notes_acceptances_user_id_version_key" ON "release_notes_acceptances"("user_id", "version");

-- CreateIndex
CREATE INDEX "release_notes_acceptances_user_id_idx" ON "release_notes_acceptances"("user_id");

-- CreateIndex
CREATE INDEX "release_notes_acceptances_version_idx" ON "release_notes_acceptances"("version");

-- AddForeignKey
ALTER TABLE "release_notes_acceptances" ADD CONSTRAINT "release_notes_acceptances_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users"("id") ON DELETE CASCADE ON UPDATE CASCADE;

