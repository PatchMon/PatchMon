import { useQuery } from "@tanstack/react-query";
import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, Legend } from "recharts";
import { complianceAPI } from "../../utils/complianceApi";

const ComplianceTrend = ({ hostId, days = 30 }) => {
	const { data: trends } = useQuery({
		queryKey: ["compliance-trends", hostId, days],
		queryFn: () => complianceAPI.getTrends(hostId, days).then((res) => res.data),
		enabled: !!hostId,
	});

	if (!trends || trends.length === 0) {
		return null;
	}

	// Check if we have both profile types
	const hasOpenSCAP = trends.some((t) => t.compliance_profiles?.type === "openscap");
	const hasDockerBench = trends.some((t) => t.compliance_profiles?.type === "docker-bench");

	// Group data by date and profile type
	const dateMap = new Map();
	trends.forEach((t) => {
		const date = new Date(t.completed_at).toLocaleDateString();
		const type = t.compliance_profiles?.type || "unknown";

		if (!dateMap.has(date)) {
			dateMap.set(date, { date, openscap: null, dockerBench: null });
		}

		const entry = dateMap.get(date);
		if (type === "openscap") {
			// Keep the latest score for each date
			entry.openscap = t.score;
		} else if (type === "docker-bench") {
			entry.dockerBench = t.score;
		}
	});

	const chartData = Array.from(dateMap.values());

	// Custom tooltip formatter
	const CustomTooltip = ({ active, payload, label }) => {
		if (!active || !payload || payload.length === 0) return null;

		return (
			<div className="bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 shadow-lg">
				<p className="text-gray-400 text-sm mb-1">{label}</p>
				{payload.map((entry, index) => (
					entry.value != null && (
						<div key={index} className="flex items-center gap-2 text-sm">
							<div
								className="w-2.5 h-2.5 rounded-full"
								style={{ backgroundColor: entry.color }}
							/>
							<span className="text-gray-300">{entry.name}:</span>
							<span className="text-white font-medium">{entry.value.toFixed(1)}%</span>
						</div>
					)
				))}
			</div>
		);
	};

	return (
		<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-4">
			<h3 className="text-white font-medium mb-4">Compliance Trend</h3>
			<div className="h-48">
				<ResponsiveContainer width="100%" height="100%">
					<LineChart data={chartData}>
						<XAxis
							dataKey="date"
							stroke="#6b7280"
							fontSize={12}
							tickLine={false}
						/>
						<YAxis
							domain={[0, 100]}
							stroke="#6b7280"
							fontSize={12}
							tickLine={false}
							tickFormatter={(v) => `${v}%`}
						/>
						<Tooltip content={<CustomTooltip />} />
						{hasOpenSCAP && (
							<Line
								type="monotone"
								dataKey="openscap"
								name="OpenSCAP"
								stroke="#22c55e"
								strokeWidth={2}
								dot={{ fill: "#22c55e", r: 4 }}
								connectNulls
							/>
						)}
						{hasDockerBench && (
							<Line
								type="monotone"
								dataKey="dockerBench"
								name="Docker Bench"
								stroke="#3b82f6"
								strokeWidth={2}
								dot={{ fill: "#3b82f6", r: 4 }}
								connectNulls
							/>
						)}
						{(hasOpenSCAP || hasDockerBench) && (
							<Legend
								wrapperStyle={{ paddingTop: "10px" }}
								formatter={(value) => <span className="text-gray-300 text-sm">{value}</span>}
							/>
						)}
					</LineChart>
				</ResponsiveContainer>
			</div>
		</div>
	);
};

export default ComplianceTrend;
