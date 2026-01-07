#!/usr/bin/env node
/**
 * Migration Script: Hash Legacy Plaintext API Keys
 *
 * This script migrates legacy plaintext API keys to bcrypt hashes.
 * It's safe to run multiple times - it only processes keys that aren't already hashed.
 *
 * IMPORTANT: After running this migration, existing agents with plaintext keys
 * will continue to work because the verifyApiKey function supports both formats.
 *
 * Usage:
 *   node scripts/migrate-api-keys.js [--dry-run]
 *
 * Options:
 *   --dry-run    Show what would be migrated without making changes
 */

const { PrismaClient } = require("@prisma/client");
const bcrypt = require("bcryptjs");

const prisma = new PrismaClient();

// Regex to detect bcrypt hashes (they start with $2a$, $2b$, or $2y$)
const BCRYPT_HASH_REGEX = /^\$2[aby]\$/;

async function migrateApiKeys(dryRun = false) {
	console.log("=".repeat(60));
	console.log("API Key Migration: Plaintext to Bcrypt Hash");
	console.log("=".repeat(60));
	console.log(`Mode: ${dryRun ? "DRY RUN (no changes will be made)" : "LIVE"}`);
	console.log("");

	try {
		// Find all hosts
		const hosts = await prisma.hosts.findMany({
			select: {
				id: true,
				friendly_name: true,
				api_id: true,
				api_key: true,
			},
		});

		console.log(`Found ${hosts.length} total hosts`);

		// Filter hosts with plaintext keys (not bcrypt hashes)
		const hostsWithPlaintextKeys = hosts.filter(
			(host) => !BCRYPT_HASH_REGEX.test(host.api_key),
		);

		console.log(
			`Found ${hostsWithPlaintextKeys.length} hosts with plaintext API keys`,
		);
		console.log("");

		if (hostsWithPlaintextKeys.length === 0) {
			console.log("All API keys are already hashed. Nothing to migrate.");
			return { migrated: 0, total: hosts.length };
		}

		// Process each host
		let migratedCount = 0;
		let errorCount = 0;

		for (const host of hostsWithPlaintextKeys) {
			try {
				console.log(`Processing: ${host.friendly_name} (${host.api_id})`);

				if (dryRun) {
					console.log(
						`  [DRY RUN] Would hash API key (current length: ${host.api_key.length} chars)`,
					);
					migratedCount++;
					continue;
				}

				// Hash the plaintext API key with bcrypt (cost factor 10)
				const hashedKey = await bcrypt.hash(host.api_key, 10);

				// Update the host with the hashed key
				await prisma.hosts.update({
					where: { id: host.id },
					data: { api_key: hashedKey },
				});

				console.log(`  Migrated successfully`);
				migratedCount++;
			} catch (error) {
				console.error(`  ERROR: ${error.message}`);
				errorCount++;
			}
		}

		console.log("");
		console.log("=".repeat(60));
		console.log("Migration Summary");
		console.log("=".repeat(60));
		console.log(`Total hosts: ${hosts.length}`);
		console.log(
			`Already hashed: ${hosts.length - hostsWithPlaintextKeys.length}`,
		);
		console.log(`Migrated: ${migratedCount}`);
		console.log(`Errors: ${errorCount}`);

		if (dryRun) {
			console.log("");
			console.log(
				"This was a dry run. Run without --dry-run to apply changes.",
			);
		}

		return { migrated: migratedCount, total: hosts.length, errors: errorCount };
	} catch (error) {
		console.error("Migration failed:", error);
		throw error;
	} finally {
		await prisma.$disconnect();
	}
}

// Parse command line arguments
const args = process.argv.slice(2);
const dryRun = args.includes("--dry-run");

// Run migration
migrateApiKeys(dryRun)
	.then((result) => {
		process.exit(result.errors > 0 ? 1 : 0);
	})
	.catch((error) => {
		console.error("Fatal error:", error);
		process.exit(1);
	});
