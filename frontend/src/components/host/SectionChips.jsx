import {
	Container,
	Cpu,
	Globe,
	Network,
	Package,
	Server,
	Shield,
} from "lucide-react";

// Icon and label mapping for each section name an agent can report on.
// New section keys: extend SECTION_META below.
const SECTION_META = {
	packages: { icon: Package, label: "Packages" },
	metrics: { icon: Cpu, label: "Metrics" },
	hostname: { icon: Server, label: "Hostname" },
	interfaces: { icon: Network, label: "Interfaces" },
	repos: { icon: Globe, label: "Repos" },
	repositories: { icon: Globe, label: "Repos" },
	compliance: { icon: Shield, label: "Compliance" },
	docker: { icon: Container, label: "Docker" },
};

const FALLBACK = { icon: Package, label: null };

const formatLabel = (key) => {
	if (!key) return "Section";
	return key
		.toString()
		.replace(/[_-]+/g, " ")
		.replace(/\s+/g, " ")
		.trim()
		.split(" ")
		.map((word) =>
			word.length === 0 ? word : word[0].toUpperCase() + word.slice(1),
		)
		.join(" ");
};

const Chip = ({ name, variant }) => {
	const meta = SECTION_META[name?.toLowerCase()] || FALLBACK;
	const Icon = meta.icon;
	const label = meta.label || formatLabel(name);

	const tooltip =
		variant === "updated"
			? `${label} updated: agent sent fresh data this cycle`
			: `${label} skipped: hash matched server's stored value, no payload sent`;

	const baseClasses =
		"inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium whitespace-nowrap";
	const updatedClasses =
		"bg-primary-100 text-primary-800 dark:bg-primary-900 dark:text-primary-200";
	const skippedClasses =
		"bg-secondary-100 text-secondary-500 dark:bg-secondary-700 dark:text-secondary-300 line-through opacity-80";

	return (
		<span
			className={`${baseClasses} ${variant === "updated" ? updatedClasses : skippedClasses}`}
			title={tooltip}
		>
			<Icon className="h-3 w-3 flex-shrink-0" />
			<span>{label}</span>
		</span>
	);
};

const SectionChips = ({ sectionsSent = [], sectionsUnchanged = [] }) => {
	const sent = Array.isArray(sectionsSent) ? sectionsSent : [];
	const skipped = Array.isArray(sectionsUnchanged) ? sectionsUnchanged : [];

	if (sent.length === 0 && skipped.length === 0) {
		return (
			<span className="text-xs text-secondary-500 dark:text-secondary-300">
				-
			</span>
		);
	}

	return (
		<div className="flex flex-wrap items-center gap-1">
			{sent.map((name) => (
				<Chip key={`sent-${name}`} name={name} variant="updated" />
			))}
			{skipped.map((name) => (
				<Chip key={`skipped-${name}`} name={name} variant="skipped" />
			))}
		</div>
	);
};

export default SectionChips;
