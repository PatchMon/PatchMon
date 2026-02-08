const { prisma } = require("./automation/shared/prisma");
const logger = require("../utils/logger");
const { v4: uuidv4 } = require("uuid");

/**
 * Alert Config Service
 * Manages alert type configuration settings
 */
class AlertConfigService {
	/**
	 * Get all alert configuration settings
	 */
	async getAlertConfig() {
		try {
			return await prisma.alert_config.findMany({
				include: {
					users_auto_assign: {
						select: {
							id: true,
							username: true,
							email: true,
							first_name: true,
							last_name: true,
						},
					},
				},
				orderBy: { alert_type: "asc" },
			});
		} catch (error) {
			logger.error(`❌ Failed to get alert config:`, error);
			throw error;
		}
	}

	/**
	 * Get configuration for specific alert type
	 */
	async getAlertConfigByType(alertType) {
		try {
			return await prisma.alert_config.findUnique({
				where: { alert_type: alertType },
				include: {
					users_auto_assign: {
						select: {
							id: true,
							username: true,
							email: true,
						},
					},
				},
			});
		} catch (error) {
			logger.error(`❌ Failed to get alert config by type:`, error);
			throw error;
		}
	}

	/**
	 * Update configuration for an alert type
	 */
	async updateAlertConfig(alertType, config) {
		try {
			const updated = await prisma.alert_config.upsert({
				where: { alert_type: alertType },
				update: {
					...config,
					updated_at: new Date(),
				},
				create: {
					id: uuidv4(),
					alert_type: alertType,
					...config,
					created_at: new Date(),
					updated_at: new Date(),
				},
			});

			logger.info(`✅ Updated alert config for type: ${alertType}`);
			return updated;
		} catch (error) {
			logger.error(`❌ Failed to update alert config:`, error);
			throw error;
		}
	}

	/**
	 * Check if alert type is enabled (used before creating alerts)
	 */
	async isAlertTypeEnabled(alertType) {
		try {
			const config = await prisma.alert_config.findUnique({
				where: { alert_type: alertType },
				select: { is_enabled: true },
			});

			// If config doesn't exist, default to enabled
			return config ? config.is_enabled : true;
		} catch (error) {
			logger.error(`❌ Failed to check if alert type is enabled:`, error);
			// Default to enabled on error
			return true;
		}
	}

	/**
	 * Get default severity for alert type
	 */
	async getDefaultSeverity(alertType) {
		try {
			const config = await prisma.alert_config.findUnique({
				where: { alert_type: alertType },
				select: { default_severity: true },
			});

			return config?.default_severity || "informational";
		} catch (error) {
			logger.error(`❌ Failed to get default severity:`, error);
			return "informational";
		}
	}

	/**
	 * Check if alert should be auto-assigned based on rules
	 */
	async shouldAutoAssign(alertType, alertData = {}) {
		try {
			const config = await prisma.alert_config.findUnique({
				where: { alert_type: alertType },
				select: {
					auto_assign_enabled: true,
					auto_assign_conditions: true,
				},
			});

			if (!config || !config.auto_assign_enabled) {
				return false;
			}

			// Check conditions if they exist
			if (config.auto_assign_conditions) {
				const conditions = config.auto_assign_conditions;

				// Check severity threshold if specified
				if (conditions.severity_threshold && alertData.severity) {
					const severityOrder = {
						informational: 0,
						warning: 1,
						error: 2,
						critical: 3,
					};

					const alertSeverity = severityOrder[alertData.severity] || 0;
					const threshold = severityOrder[conditions.severity_threshold] || 0;

					if (alertSeverity < threshold) {
						return false;
					}
				}
			}

			return true;
		} catch (error) {
			logger.error(`❌ Failed to check auto-assign:`, error);
			return false;
		}
	}

	/**
	 * Get user to auto-assign to based on rules
	 */
	async getAutoAssignUser(alertType) {
		try {
			const config = await prisma.alert_config.findUnique({
				where: { alert_type: alertType },
				select: {
					auto_assign_enabled: true,
					auto_assign_rule: true,
					auto_assign_user_id: true,
				},
			});

			if (!config || !config.auto_assign_enabled) {
				return null;
			}

			// Handle different assignment rules
			if (config.auto_assign_rule === "specific_user" && config.auto_assign_user_id) {
				return config.auto_assign_user_id;
			}

			// For "first_available" or "round_robin", we'd need additional logic
			// For now, return the specific user if set
			return config.auto_assign_user_id || null;
		} catch (error) {
			logger.error(`❌ Failed to get auto-assign user:`, error);
			return null;
		}
	}

	/**
	 * Get alerts that should be cleaned up based on retention policies
	 */
	async getAlertsToCleanup() {
		try {
			const configs = await prisma.alert_config.findMany({
				where: {
					retention_days: { not: null },
				},
			});

			const alertsToCleanup = [];

			for (const config of configs) {
				const cutoffDate = new Date();
				cutoffDate.setDate(cutoffDate.getDate() - config.retention_days);

				const where = {
					alert_type: config.alert_type,
					created_at: { lte: cutoffDate },
				};

				if (config.cleanup_resolved_only) {
					where.is_active = false;
					where.resolved_at = { not: null };
				} else {
					where.is_active = false;
				}

				const alerts = await prisma.alerts.findMany({
					where,
					select: { id: true, type: true, created_at: true },
				});

				alertsToCleanup.push(...alerts);
			}

			return alertsToCleanup;
		} catch (error) {
			logger.error(`❌ Failed to get alerts to cleanup:`, error);
			throw error;
		}
	}

	/**
	 * Perform cleanup of old alerts based on retention policies
	 */
	async cleanupOldAlerts() {
		try {
			const alertsToCleanup = await this.getAlertsToCleanup();
			let deletedCount = 0;

			for (const alert of alertsToCleanup) {
				try {
					// Delete alert (cascade will delete history)
					await prisma.alerts.delete({
						where: { id: alert.id },
					});
					deletedCount++;
				} catch (error) {
					logger.error(`Failed to delete alert ${alert.id}:`, error);
				}
			}

			logger.info(`✅ Cleaned up ${deletedCount} old alerts`);
			return { deleted_count: deletedCount };
		} catch (error) {
			logger.error(`❌ Failed to cleanup old alerts:`, error);
			throw error;
		}
	}

	/**
	 * Check if alert should be escalated
	 */
	async checkEscalationRules(alertId) {
		try {
			const alert = await prisma.alerts.findUnique({
				where: { id: alertId },
				select: {
					type: true,
					created_at: true,
				},
			});

			if (!alert) {
				return false;
			}

			const config = await prisma.alert_config.findUnique({
				where: { alert_type: alert.type },
				select: {
					escalation_enabled: true,
					escalation_after_hours: true,
				},
			});

			if (!config || !config.escalation_enabled || !config.escalation_after_hours) {
				return false;
			}

			const hoursSinceCreation =
				(Date.now() - new Date(alert.created_at).getTime()) / (1000 * 60 * 60);

			return hoursSinceCreation >= config.escalation_after_hours;
		} catch (error) {
			logger.error(`❌ Failed to check escalation rules:`, error);
			return false;
		}
	}
}

// Export singleton instance
module.exports = new AlertConfigService();

