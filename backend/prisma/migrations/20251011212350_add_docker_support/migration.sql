-- CreateTable
CREATE TABLE "docker_images" (
    "id" TEXT NOT NULL,
    "repository" TEXT NOT NULL,
    "tag" TEXT NOT NULL DEFAULT 'latest',
    "image_id" TEXT NOT NULL,
    "digest" TEXT,
    "size_bytes" BIGINT,
    "source" TEXT NOT NULL DEFAULT 'docker-hub',
    "created_at" TIMESTAMP(3) NOT NULL,
    "last_pulled" TIMESTAMP(3),
    "last_checked" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "docker_images_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "docker_containers" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "container_id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "image_id" TEXT,
    "image_name" TEXT NOT NULL,
    "image_tag" TEXT NOT NULL DEFAULT 'latest',
    "status" TEXT NOT NULL,
    "state" TEXT,
    "ports" JSONB,
    "created_at" TIMESTAMP(3) NOT NULL,
    "started_at" TIMESTAMP(3),
    "updated_at" TIMESTAMP(3) NOT NULL,
    "last_checked" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "docker_containers_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "docker_image_updates" (
    "id" TEXT NOT NULL,
    "image_id" TEXT NOT NULL,
    "current_tag" TEXT NOT NULL,
    "available_tag" TEXT NOT NULL,
    "is_security_update" BOOLEAN NOT NULL DEFAULT false,
    "severity" TEXT,
    "changelog_url" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "docker_image_updates_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE INDEX "docker_images_repository_idx" ON "docker_images"("repository");

-- CreateIndex
CREATE INDEX "docker_images_source_idx" ON "docker_images"("source");

-- CreateIndex
CREATE INDEX "docker_images_repository_tag_idx" ON "docker_images"("repository", "tag");

-- CreateIndex
CREATE UNIQUE INDEX "docker_images_repository_tag_image_id_key" ON "docker_images"("repository", "tag", "image_id");

-- CreateIndex
CREATE INDEX "docker_containers_host_id_idx" ON "docker_containers"("host_id");

-- CreateIndex
CREATE INDEX "docker_containers_image_id_idx" ON "docker_containers"("image_id");

-- CreateIndex
CREATE INDEX "docker_containers_status_idx" ON "docker_containers"("status");

-- CreateIndex
CREATE INDEX "docker_containers_name_idx" ON "docker_containers"("name");

-- CreateIndex
CREATE UNIQUE INDEX "docker_containers_host_id_container_id_key" ON "docker_containers"("host_id", "container_id");

-- CreateIndex
CREATE INDEX "docker_image_updates_image_id_idx" ON "docker_image_updates"("image_id");

-- CreateIndex
CREATE INDEX "docker_image_updates_is_security_update_idx" ON "docker_image_updates"("is_security_update");

-- CreateIndex
CREATE UNIQUE INDEX "docker_image_updates_image_id_available_tag_key" ON "docker_image_updates"("image_id", "available_tag");

-- AddForeignKey
ALTER TABLE "docker_containers" ADD CONSTRAINT "docker_containers_image_id_fkey" FOREIGN KEY ("image_id") REFERENCES "docker_images"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "docker_image_updates" ADD CONSTRAINT "docker_image_updates_image_id_fkey" FOREIGN KEY ("image_id") REFERENCES "docker_images"("id") ON DELETE CASCADE ON UPDATE CASCADE;

