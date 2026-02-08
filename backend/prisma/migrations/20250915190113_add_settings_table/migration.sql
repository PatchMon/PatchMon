-- CreateTable
CREATE TABLE "settings" (
    "id" TEXT NOT NULL,
    "server_url" TEXT NOT NULL DEFAULT 'http://localhost:3001',
    "server_protocol" TEXT NOT NULL DEFAULT 'http',
    "server_host" TEXT NOT NULL DEFAULT 'localhost',
    "server_port" INTEGER NOT NULL DEFAULT 3001,
    "frontend_url" TEXT NOT NULL DEFAULT 'http://localhost:3000',
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "settings_pkey" PRIMARY KEY ("id")
);
