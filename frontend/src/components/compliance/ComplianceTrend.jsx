import { useQuery } from "@tanstack/react-query";
import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer } from "recharts";
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

	const chartData = trends.map((t) => ({
		date: new Date(t.completed_at).toLocaleDateString(),
		score: t.score,
	}));

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
						<Tooltip
							contentStyle={{
								backgroundColor: "#1f2937",
								border: "1px solid #374151",
								borderRadius: "0.5rem",
							}}
							labelStyle={{ color: "#9ca3af" }}
							formatter={(value) => [`${value.toFixed(1)}%`, "Score"]}
						/>
						<Line
							type="monotone"
							dataKey="score"
							stroke="#3b82f6"
							strokeWidth={2}
							dot={{ fill: "#3b82f6", r: 4 }}
						/>
					</LineChart>
				</ResponsiveContainer>
			</div>
		</div>
	);
};

export default ComplianceTrend;
