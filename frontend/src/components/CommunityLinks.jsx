/**
 * Shared community/social links component.
 * Fetches links from GET /api/v1/community/links and renders with icons.
 * Used in: Layout (top nav), Login (footer), FirstTimeWizard (Get in Touch step).
 */

import { useQuery } from "@tanstack/react-query";
import { BookOpen, Github, Globe, Mail, Route, Star } from "lucide-react";
import { FaLinkedin, FaYoutube } from "react-icons/fa";
import { communityAPI } from "../utils/api";
import BuyMeACoffeeIcon from "./BuyMeACoffeeIcon";
import DiscordIcon from "./DiscordIcon";

const ICON_MAP = {
	discord: DiscordIcon,
	github: Github,
	github_issues: Github,
	email: Mail,
	linkedin: FaLinkedin,
	youtube: FaYoutube,
	buymeacoffee: BuyMeACoffeeIcon,
	roadmap: Route,
	docs: BookOpen,
	website: Globe,
};

const ICON_COLORS = {
	discord: "text-[#5865F2]",
	github: "",
	github_issues: "",
	email: "",
	linkedin: "text-[#0077B5]",
	youtube: "text-[#FF0000]",
	buymeacoffee: "text-yellow-500",
	roadmap: "",
	docs: "",
	website: "",
};

const CommunityLinksGrid = ({
	links,
	className = "",
	linkClassName = "",
	showStats = true,
	iconOnlyIds = [],
}) => {
	if (!links?.length) return null;

	return (
		<div className={`flex flex-wrap items-center gap-2 ${className}`}>
			{links.map((link) => {
				const IconComponent = ICON_MAP[link.id] || null;
				const colorClass = ICON_COLORS[link.id] || "";
				const isMailto = link.url?.startsWith("mailto:");
				const iconOnly = iconOnlyIds.includes(link.id);

				return (
					<a
						key={link.id}
						href={link.url}
						target={isMailto ? undefined : "_blank"}
						rel={isMailto ? undefined : "noopener noreferrer"}
						className={`flex items-center justify-center gap-1.5 px-3 h-10 rounded-lg transition-colors border border-secondary-200 dark:border-secondary-600 bg-white/50 dark:bg-secondary-800/50 hover:bg-secondary-100 dark:hover:bg-secondary-700 text-secondary-700 dark:text-secondary-200 hover:text-secondary-900 dark:hover:text-white ${linkClassName}`}
						title={link.label}
					>
						{IconComponent && (
							<IconComponent
								className={`h-5 w-5 flex-shrink-0 ${colorClass}`}
							/>
						)}
						{!iconOnly && (
							<span className="text-sm font-medium truncate max-w-[120px]">
								{link.label}
							</span>
						)}
						{showStats && link.stat && link.statLabel === "stars" && (
							<div className="flex items-center gap-0.5">
								<Star className="h-4 w-4 fill-current text-yellow-500 flex-shrink-0" />
								<span className="text-sm">{link.stat}</span>
							</div>
						)}
						{showStats && link.stat && link.statLabel === "members" && (
							<span className="text-sm">{link.stat}</span>
						)}
						{showStats && link.stat && !link.statLabel && (
							<span className="text-sm truncate max-w-[80px]">{link.stat}</span>
						)}
					</a>
				);
			})}
		</div>
	);
};

/**
 * Hook to fetch community links. Returns { links, isLoading }.
 */
export const useCommunityLinks = () => {
	const { data, isLoading } = useQuery({
		queryKey: ["communityLinks"],
		queryFn: () => communityAPI.getLinks(),
		staleTime: 5 * 60 * 1000, // 5 min cache
	});
	return { links: data?.links ?? [], isLoading };
};

/**
 * Renders community links for the wizard "Get in Touch" step.
 * Layout: Row 1 = Discord, GitHub (icons + stats only) | Row 2 = LinkedIn, YouTube
 * Bottom: resource links (icons only)
 */
export const WizardCommunityLinks = () => {
	const { links, isLoading } = useCommunityLinks();

	if (isLoading) {
		return (
			<div className="flex justify-center py-8">
				<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
			</div>
		);
	}

	const row1 = links.filter((l) => ["discord", "github"].includes(l.id));
	const row2 = links.filter((l) => ["linkedin", "youtube"].includes(l.id));
	const others = links.filter(
		(l) => !["discord", "github", "linkedin", "youtube"].includes(l.id),
	);

	return (
		<div className="space-y-4">
			<div className="flex flex-wrap justify-center gap-3">
				<CommunityLinksGrid
					links={row1}
					linkClassName="min-w-[140px] justify-center"
					showStats={true}
					iconOnlyIds={["discord", "github"]}
				/>
			</div>
			<div className="flex flex-wrap justify-center gap-3">
				<CommunityLinksGrid
					links={row2}
					linkClassName="min-w-[140px] justify-center"
					showStats={true}
					iconOnlyIds={["linkedin", "youtube"]}
				/>
			</div>
			{others.length > 0 && (
				<div className="flex flex-wrap justify-center gap-2 pt-2 border-t border-secondary-200 dark:border-secondary-600">
					<CommunityLinksGrid
						links={others}
						showStats={false}
						iconOnlyIds={others.map((l) => l.id)}
					/>
				</div>
			)}
		</div>
	);
};

/**
 * Renders community links for the Login page footer.
 * Uses same API, different layout/styling for dark overlay.
 */
export const LoginCommunityLinks = () => {
	const { links, isLoading } = useCommunityLinks();

	if (isLoading) return null;

	const navIds = [
		"github",
		"buymeacoffee",
		"discord",
		"linkedin",
		"youtube",
		"roadmap",
		"docs",
		"website",
	];
	const filtered = links.filter((l) => navIds.includes(l.id));

	return (
		<div className="flex flex-wrap items-center gap-2">
			{filtered.map((link) => {
				const IconComponent = ICON_MAP[link.id];
				const colorClass = ICON_COLORS[link.id] || "";
				const isMailto = link.url?.startsWith("mailto:");

				return (
					<a
						key={link.id}
						href={link.url}
						target={isMailto ? undefined : "_blank"}
						rel={isMailto ? undefined : "noopener noreferrer"}
						className="flex items-center justify-center gap-1.5 px-3 h-10 bg-white/10 hover:bg-white/20 backdrop-blur-sm rounded-lg transition-colors border border-white/10"
						title={link.label}
					>
						{IconComponent && (
							<IconComponent
								className={`h-5 w-5 flex-shrink-0 ${
									link.id === "discord" ||
									link.id === "linkedin" ||
									link.id === "youtube"
										? colorClass
										: "text-white"
								}`}
							/>
						)}
						{link.statLabel === "stars" && (
							<>
								<Star className="h-3.5 w-3.5 fill-current text-yellow-400" />
								<span className="text-sm font-medium text-white">
									{link.stat}
								</span>
							</>
						)}
						{link.statLabel === "members" && link.stat && (
							<span className="text-sm font-medium text-white">
								{link.stat}
							</span>
						)}
						{!link.statLabel && link.stat && link.id !== "email" && (
							<span className="text-sm font-medium text-white">
								{link.stat}
							</span>
						)}
						{!link.stat &&
							["roadmap", "docs", "website", "buymeacoffee"].includes(
								link.id,
							) && <span className="sr-only">{link.label}</span>}
					</a>
				);
			})}
		</div>
	);
};

export default CommunityLinksGrid;
