// Mirrors classifyComplianceScan in server-source-code/internal/store/compliance_dashboard.go.
export const hostComplianceStatus = (row) => {
	if (row?.compliance_status) return row.compliance_status;
	const score = Number(row?.score);
	if (score >= 80) return "compliant";
	if (score >= 60 || !(Number(row?.failed) > 0)) return "warning";
	return "critical";
};
