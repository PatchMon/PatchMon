# Automatic Compliance Scan Cleanup

## Summary
Adds automated cleanup for stalled compliance scans that have been running for more than 3 hours.

## What's New

### Backend
- **New Automation Service**: `ComplianceScanCleanup` automatically terminates scans running over 3 hours
- **Schedule**: Runs daily at 1 AM
- **Manual Trigger**: New endpoint `POST /api/v1/compliance/scans/cleanup` for on-demand cleanup
- **Monitoring**: New endpoint `GET /api/v1/compliance/scans/stalled` to view stuck scans without cleaning them

### Frontend
- **Automation Page**: Compliance Scan Cleanup now appears in the Automation Management page
- **Manual Trigger**: Users can manually trigger cleanup via the "Run Now" button
- **Schedule Display**: Added support for "Daily at 1 AM" schedule display

## Changes
- Compliance scans running over 3 hours are automatically marked as failed
- Cleanup runs daily at 1 AM (configurable)
- Manual cleanup can be triggered from the Automation page
- Detailed logging of terminated scans with runtime information

## Benefits
- Prevents database bloat from stuck compliance scans
- Improves system performance by cleaning up long-running processes
- Provides visibility into scan health through the stalled scans endpoint
