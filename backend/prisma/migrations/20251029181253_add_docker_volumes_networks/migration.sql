-- CreateTable
CREATE TABLE "docker_volumes" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "volume_id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "driver" TEXT NOT NULL,
    "mountpoint" TEXT,
    "renderer" TEXT,
    "scope" TEXT NOT NULL DEFAULT 'local',
    "labels" JSONB,
    "options" JSONB,
    "size_bytes" BIGINT,
    "ref_count" INTEGER NOT NULL DEFAULT 0,
    "created_at" TIMESTAMP(3) NOT NULL,
    "updated_at" TIMESTAMP(3) NOT NULL,
    "last_checked" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "docker_volumes_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "docker_networks" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "network_id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "driver" TEXT NOT NULL,
    "scope" TEXT NOT NULL DEFAULT 'local',
    "ipv6_enabled" BOOLEAN NOT NULL DEFAULT false,
    "internal" BOOLEAN NOT NULL DEFAULT false,
    "attachable" BOOLEAN NOT NULL DEFAULT true,
    "ingress" BOOLEAN NOT NULL DEFAULT false,
    "config_only" BOOLEAN NOT NULL DEFAULT false,
    "labels" JSONB,
    "ipam" JSONB,
    "container_count" INTEGER NOT NULL DEFAULT 0,
    "created_at" TIMESTAMP(3),
    "updated_at" TIMESTAMP(3) NOT NULL,
    "last_checked" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "docker_networks_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE INDEX "docker_volumes_host_id_idx" ON "docker_volumes"("host_id");

-- CreateIndex
CREATE INDEX "docker_volumes_name_idx" ON "docker_volumes"("name");

-- CreateIndex
CREATE INDEX "docker_volumes_driver_idx" ON "docker_volumes"("driver");

-- CreateIndex
CREATE UNIQUE INDEX "docker_volumes_host_id_volume_id_key" ON "docker_volumes"("host_id", "volume_id");

-- CreateIndex
CREATE INDEX "docker_networks_host_id_idx" ON "docker_networks"("host_id");

-- CreateIndex
CREATE INDEX "docker_networks_name_idx" ON "docker_networks"("name");

-- CreateIndex
CREATE INDEX "docker_networks_driver_idx" ON "docker_networks"("driver");

-- CreateIndex
CREATE UNIQUE INDEX "docker_networks_host_id_network_id_key" ON "docker_networks"("host_id", "network_id");

-- AddForeignKey
ALTER TABLE "docker_volumes" ADD CONSTRAINT "docker_volumes_host_id_fkey" FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "docker_networks" ADD CONSTRAINT "docker_networks_host_id_fkey" FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE CASCADE ON UPDATE CASCADE;

