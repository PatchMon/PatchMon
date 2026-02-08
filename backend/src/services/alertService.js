const { prisma } = require("./automation/shared/prisma");
const logger = require("../utils/logger");
const { v4: uuidv4 } = require("uuid");

/**
 * Alert Service
 * Manages alerts, alert history, and alert actions
 */
class AlertService {
	/**
	 * Check if alerts system is enabled
	 */
	async isAlertsEnabled() {
		try {
			const settings = await prisma.settings.findFirst({
				select: { alerts_enabled: true },
			});
			return settings?.alerts_enabled !== false; // Default to true if not set
		} catch (error) {
			logger.error(`❌ Failed to check if alerts are enabled:`, error);
			return true; // Default to enabled on error
		}
	}

	/**
	 * Create a new alert
	 */
	async createAlert(type, severity, title, message, metadata = null) {
		try {
			// Check if alerts system is enabled
			const alertsEnabled = await this.isAlertsEnabled();
			if (!alertsEnabled) {
				logger.info(`⚠️ Alerts system is disabled, skipping alert creation: ${type}`);
				return null;
			}

			const alert = await prisma.alerts.create({
				data: {
					id: uuidv4(),
					type,
					severity,
					title,
					message,
					metadata: metadata || {},
					is_active: true,
					created_at: new Date(),
					updated_at: new Date(),
				},
			});

			// Record "created" action in history
			await this.recordAlertHistory(alert.id, null, "created", {
				system_action: true,
			});

			logger.info(`✅ Created alert: ${alert.id} (${type})`);
			return alert;
		} catch (error) {
			logger.error(`❌ Failed to create alert:`, error);
			throw error;
		}
	}

	/**
	 * Update an alert
	 */
	async updateAlert(alertId, data) {
		try {
			const alert = await prisma.alerts.update({
				where: { id: alertId },
				data: {
					...data,
					updated_at: new Date(),
				},
			});

			logger.info(`✅ Updated alert: ${alertId}`);
			return alert;
		} catch (error) {
			logger.error(`❌ Failed to update alert ${alertId}:`, error);
			throw error;
		}
	}

	/**
	 * Get all active alerts with current state derived from latest history entry
	 */
	async getActiveAlerts(userId = null, includeInactive = true) {
		try {
			const where = {};

			// Only filter by is_active if we don't want inactive alerts
			if (!includeInactive) {
				where.is_active = true;
			}

			if (userId) {
				where.assigned_to_user_id = userId;
			}

			const alerts = await prisma.alerts.findMany({
				where,
				include: {
					users_assigned: {
						select: {
							id: true,
							username: true,
							email: true,
							first_name: true,
							last_name: true,
						},
					},
					users_resolved: {
						select: {
							id: true,
							username: true,
						},
					},
					alert_history: {
						orderBy: { created_at: "desc" },
						take: 1,
						include: {
							users: {
								select: {
									id: true,
									username: true,
									email: true,
								},
							},
						},
					},
				},
				orderBy: { created_at: "desc" },
			});

			// Enrich alerts with current state from latest history
			return alerts.map((alert) => {
				const latestHistory = alert.alert_history[0];
				const currentState = latestHistory
					? {
							action: latestHistory.action,
							user: latestHistory.users,
							timestamp: latestHistory.created_at,
						}
					: null;

				return {
					...alert,
					current_state: currentState,
				};
			});
		} catch (error) {
			logger.error(`❌ Failed to get active alerts:`, error);
			throw error;
		}
	}

	/**
	 * Get alerts by severity
	 */
	async getAlertsBySeverity(severity) {
		try {
			return await prisma.alerts.findMany({
				where: {
					is_active: true,
					severity,
				},
				orderBy: { created_at: "desc" },
			});
		} catch (error) {
			logger.error(`❌ Failed to get alerts by severity:`, error);
			throw error;
		}
	}

	/**
	 * Get alerts assigned to a specific user
	 */
	async getAlertsAssignedToUser(userId) {
		try {
			return await prisma.alerts.findMany({
				where: {
					is_active: true,
					assigned_to_user_id: userId,
				},
				include: {
					users_assigned: {
						select: {
							id: true,
							username: true,
							email: true,
						},
					},
				},
				orderBy: { created_at: "desc" },
			});
		} catch (error) {
			logger.error(`❌ Failed to get alerts assigned to user:`, error);
			throw error;
		}
	}

	/**
	 * Get current state of alert from latest history entry
	 */
	async getAlertCurrentState(alertId) {
		try {
			const latestHistory = await prisma.alert_history.findFirst({
				where: { alert_id: alertId },
				orderBy: { created_at: "desc" },
				include: {
					users: {
						select: {
							id: true,
							username: true,
							email: true,
						},
					},
				},
			});

			return latestHistory
				? {
						action: latestHistory.action,
						user: latestHistory.users,
						timestamp: latestHistory.created_at,
						metadata: latestHistory.metadata,
					}
				: null;
		} catch (error) {
			logger.error(`❌ Failed to get alert current state:`, error);
			throw error;
		}
	}

	/**
	 * Perform action on alert (silence, done, assign, etc.), records in history
	 */
	async performAlertAction(userId, alertId, actionName, metadata = null) {
		try {
			// Verify action exists
			const action = await prisma.alert_actions.findUnique({
				where: { name: actionName },
			});

			if (!action) {
				throw new Error(`Invalid action: ${actionName}`);
			}

			logger.info(`[alert-service] Performing action ${actionName} on alert ${alertId} by ${userId || 'system'}`);

			// Record in history first
			await this.recordAlertHistory(alertId, userId, actionName, metadata);
			logger.info(`[alert-service] History recorded for alert ${alertId}`);

			// Handle specific actions that update alert fields
			if (actionName === "resolved" || actionName === "done") {
				// These actions hide the alert from stats (mark as inactive/resolved)
				logger.info(`[alert-service] Marking alert ${alertId} as ${actionName} - setting is_active=false, resolved_at=now`);
				await prisma.alerts.update({
					where: { id: alertId },
					data: {
						is_active: false,
						resolved_at: new Date(),
						resolved_by_user_id: userId,
						updated_at: new Date(),
					},
				});
				logger.info(`[alert-service] ✅ Alert ${alertId} marked as ${actionName}`);
			} else if (["assigned", "silenced", "unsilenced", "acknowledged"].includes(actionName)) {
				// These actions should un-resolve the alert (make it active again)
				// This allows users to take action on a resolved alert and have it show up in stats again
				logger.info(`[alert-service] Un-resolving alert ${alertId} via action ${actionName} - setting is_active=true, resolved_at=null`);
				await prisma.alerts.update({
					where: { id: alertId },
					data: {
						is_active: true,
						resolved_at: null,
						resolved_by_user_id: null,
						updated_at: new Date(),
					},
				});
				logger.info(`[alert-service] ✅ Alert ${alertId} un-resolved via action ${actionName}`);
			}

			logger.info(
				`✅ Performed action ${actionName} on alert ${alertId} by ${userId || 'system'}`,
			);
			return { success: true, action: actionName };
		} catch (error) {
			logger.error(`❌ Failed to perform alert action ${actionName} on alert ${alertId}:`, error);
			logger.error(`❌ Error stack:`, error.stack);
			throw error;
		}
	}

	/**
	 * Assign alert to user, updates assigned_to_user_id and records in history
	 */
	async assignAlertToUser(alertId, assignedToUserId, assignedByUserId) {
		try {
			logger.info(`[alert-service] Assigning alert ${alertId} to user ${assignedToUserId} by ${assignedByUserId || 'system'}`);
			
			await prisma.alerts.update({
				where: { id: alertId },
				data: {
					assigned_to_user_id: assignedToUserId,
					is_active: true, // Ensure alert is active when assigned
					resolved_at: null, // Clear resolved_at when assigning (un-resolve if it was resolved)
					resolved_by_user_id: null, // Clear resolved_by_user_id when un-resolving
					updated_at: new Date(),
				},
			});
			logger.info(`[alert-service] Updated alert ${alertId} with assigned_to_user_id=${assignedToUserId}, is_active=true, resolved_at=null`);

			await this.recordAlertHistory(alertId, assignedByUserId, "assigned", {
				assigned_to_user_id: assignedToUserId,
			});
			logger.info(`[alert-service] Recorded assignment history for alert ${alertId}`);

			logger.info(
				`✅ Assigned alert ${alertId} to user ${assignedToUserId} by ${assignedByUserId || 'system'}`,
			);
			return { success: true };
		} catch (error) {
			logger.error(`❌ Failed to assign alert ${alertId} to user ${assignedToUserId}:`, error);
			logger.error(`❌ Assignment error stack:`, error.stack);
			throw error;
		}
	}

	/**
	 * Remove assignment from alert, records in history
	 */
	async unassignAlert(alertId, unassignedByUserId) {
		try {
			await prisma.alerts.update({
				where: { id: alertId },
				data: {
					assigned_to_user_id: null,
					updated_at: new Date(),
				},
			});

			await this.recordAlertHistory(alertId, unassignedByUserId, "unassigned", {
				unassigned: true,
			});

			logger.info(
				`✅ Unassigned alert ${alertId} by user ${unassignedByUserId}`,
			);
			return { success: true };
		} catch (error) {
			logger.error(`❌ Failed to unassign alert:`, error);
			throw error;
		}
	}

	/**
	 * Get alert statistics by severity
	 */
	async getAlertStats() {
		try {
			// Count all alerts that are not resolved
			// This includes alerts that are silenced or done, as severity is about the alert itself
			// We check both is_active and resolved_at to ensure we don't count resolved alerts
			const stats = await prisma.alerts.groupBy({
				by: ["severity"],
				where: {
					is_active: true, // Only count active (non-resolved) alerts
					resolved_at: null, // Also ensure resolved_at is null (double-check)
				},
				_count: {
					id: true,
				},
			});

			const result = {
				informational: 0,
				warning: 0,
				error: 0,
				critical: 0,
				total: 0,
			};

			stats.forEach((stat) => {
				if (!stat.severity) {
					logger.warn(`[alert-stats] Skipping stat with null/undefined severity:`, stat);
					return; // Skip if severity is null or undefined
				}
				const severity = stat.severity.toLowerCase().trim();
				if (result.hasOwnProperty(severity)) {
					result[severity] = stat._count.id;
					result.total += stat._count.id;
				} else {
					// Log unexpected severity values for debugging
					logger.warn(`⚠️ Unexpected severity value in stats: "${stat.severity}" (normalized: "${severity}")`);
				}
			});

			return result;
		} catch (error) {
			logger.error(`❌ Failed to get alert stats:`, error);
			throw error;
		}
	}

	/**
	 * Get history of actions for a specific alert
	 */
	async getAlertHistory(alertId) {
		try {
			return await prisma.alert_history.findMany({
				where: { alert_id: alertId },
				include: {
					users: {
						select: {
							id: true,
							username: true,
							email: true,
							first_name: true,
							last_name: true,
						},
					},
				},
				orderBy: { created_at: "desc" },
			});
		} catch (error) {
			logger.error(`❌ Failed to get alert history:`, error);
			throw error;
		}
	}

	/**
	 * Record user action in history
	 */
	async recordAlertHistory(alertId, userId, actionName, metadata = null) {
		try {
			// userId can be null for system actions (e.g., "created", "resolved" by system)
			// When using Prisma relations, we must use relation syntax, not direct foreign keys
			const data = {
				id: uuidv4(),
				alerts: {
					connect: { id: alertId }, // Connect to existing alert
				},
				action: actionName,
				metadata: metadata || {},
				created_at: new Date(),
			};

			// Only include users relation if userId is provided
			if (userId) {
				data.users = {
					connect: { id: userId },
				};
			}
			// If userId is null, we don't include the users field at all (it's nullable)

			const history = await prisma.alert_history.create({
				data,
			});

			return history;
		} catch (error) {
			logger.error(`❌ Failed to record alert history:`, error);
			throw error;
		}
	}

	/**
	 * Get list of available actions from alert_actions table
	 */
	async getAvailableActions() {
		try {
			return await prisma.alert_actions.findMany({
				orderBy: { display_name: "asc" },
			});
		} catch (error) {
			logger.error(`❌ Failed to get available actions:`, error);
			throw error;
		}
	}

	/**
	 * Initialize default actions (should be called during migration/seed)
	 */
	async initializeDefaultActions() {
		try {
			const defaultActions = [
				{
					name: "created",
					display_name: "Created",
					description: "Alert was created (system action)",
					is_state_action: false,
				},
				{
					name: "acknowledged",
					display_name: "Acknowledged",
					description: "User acknowledged the alert",
					is_state_action: false,
				},
				{
					name: "assigned",
					display_name: "Assigned",
					description: "Alert was assigned/delegated to a user",
					is_state_action: true,
				},
				{
					name: "unassigned",
					display_name: "Unassigned",
					description: "Alert assignment was removed",
					is_state_action: true,
				},
				{
					name: "silenced",
					display_name: "Silenced",
					description: "Alert was silenced",
					is_state_action: true,
				},
				{
					name: "unsilenced",
					display_name: "Unsilenced",
					description: "Alert was unsilenced",
					is_state_action: true,
				},
				{
					name: "done",
					display_name: "Mark as Done",
					description: "Alert was marked as done",
					is_state_action: true,
				},
				{
					name: "resolved",
					display_name: "Resolved",
					description: "Alert was resolved",
					is_state_action: true,
				},
				{
					name: "updated",
					display_name: "Updated",
					description: "Alert was updated (system action)",
					is_state_action: false,
				},
			];

			for (const action of defaultActions) {
				await prisma.alert_actions.upsert({
					where: { name: action.name },
					update: action,
					create: {
						id: uuidv4(),
						...action,
						created_at: new Date(),
						updated_at: new Date(),
					},
				});
			}

			logger.info("✅ Initialized default alert actions");
		} catch (error) {
			logger.error(`❌ Failed to initialize default actions:`, error);
			throw error;
		}
	}

	/**
	 * Delete an alert and its associated history (cascading delete)
	 */
	async deleteAlert(alertId) {
		try {
			// Prisma will automatically delete related alert_history records due to onDelete: Cascade
			await prisma.alerts.delete({
				where: { id: alertId },
			});

			logger.info(`✅ Deleted alert: ${alertId}`);
			return { success: true };
		} catch (error) {
			logger.error(`❌ Failed to delete alert:`, error);
			throw error;
		}
	}

	/**
	 * Delete multiple alerts and their associated history
	 */
	async deleteAlerts(alertIds) {
		try {
			if (!Array.isArray(alertIds) || alertIds.length === 0) {
				throw new Error("alertIds must be a non-empty array");
			}

			// Prisma will automatically delete related alert_history records due to onDelete: Cascade
			const result = await prisma.alerts.deleteMany({
				where: {
					id: {
						in: alertIds,
					},
				},
			});

			logger.info(`✅ Deleted ${result.count} alerts`);
			return { success: true, deletedCount: result.count };
		} catch (error) {
			logger.error(`❌ Failed to delete alerts:`, error);
			throw error;
		}
	}
}

// Export singleton instance
module.exports = new AlertService();

