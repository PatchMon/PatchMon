import { useMemo } from "react";

const defaultClassName =
	"w-full rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2";

export function TimezoneSelect({ value, onChange, className }) {
	const groups = useMemo(() => {
		const timezones = Intl.supportedValuesOf("timeZone");
		const grouped = {};
		for (const tz of timezones) {
			const slashIdx = tz.indexOf("/");
			if (slashIdx === -1) continue;
			const region = tz.slice(0, slashIdx);
			if (!grouped[region]) grouped[region] = [];
			grouped[region].push(tz);
		}
		return Object.entries(grouped).sort(([a], [b]) => a.localeCompare(b));
	}, []);

	return (
		<select
			value={value || "UTC"}
			onChange={onChange}
			className={className ?? defaultClassName}
		>
			<option value="UTC">UTC</option>
			{groups.map(([region, timezones]) => (
				<optgroup key={region} label={region}>
					{timezones.map((tz) => (
						<option key={tz} value={tz}>
							{tz.slice(tz.indexOf("/") + 1)}
						</option>
					))}
				</optgroup>
			))}
		</select>
	);
}
