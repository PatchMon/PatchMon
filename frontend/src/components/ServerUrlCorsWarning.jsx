import { useQuery } from "@tanstack/react-query";
import { AlertTriangle } from "lucide-react";
import { settingsAPI } from "../utils/api";

const OPERATOR_GUIDE_URL = "https://patchmon.net/docs/patchmon-operator-guide";

const ServerUrlCorsWarning = ({ enabled = true }) => {
	const { data } = useQuery({
		queryKey: ["serverUrlCorsCheck"],
		queryFn: () => settingsAPI.checkServerUrlCors().then((res) => res.data),
		enabled,
		retry: false,
	});

	if (!data || data.allowed !== false) return null;

	const message =
		data.reason === "server_url_invalid"
			? "The saved Server URL is not a valid URL, so agents cannot use it to connect. Check its protocol, host and port."
			: "The hostname in this Server URL is not listed in CORS_ORIGIN, so agents that use it will be rejected. Add it to CORS_ORIGIN or change the Server URL.";

	return (
		<div
			role="alert"
			className="bg-warning-50 dark:bg-warning-900/20 border border-warning-200 dark:border-warning-700 rounded-md p-4"
		>
			<div className="flex">
				<AlertTriangle className="h-5 w-5 flex-shrink-0 text-warning-500 dark:text-warning-400" />
				<div className="ml-3">
					<h3 className="text-sm font-medium text-warning-800 dark:text-warning-200">
						Agents may fail to install
					</h3>
					<p className="mt-1 text-sm text-warning-700 dark:text-warning-300">
						{message}
					</p>
					<a
						href={OPERATOR_GUIDE_URL}
						target="_blank"
						rel="noopener noreferrer"
						className="mt-2 inline-block text-sm font-medium text-warning-800 dark:text-warning-200 underline"
					>
						Operator guide
					</a>
				</div>
			</div>
		</div>
	);
};

export default ServerUrlCorsWarning;
