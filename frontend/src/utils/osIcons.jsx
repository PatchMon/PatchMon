import { Monitor } from "lucide-react";
import { DiWindows } from "react-icons/di";
// Import OS icons from react-icons Simple Icons - using only confirmed available icons
import {
	SiAlmalinux,
	SiAlpinelinux,
	SiArchlinux,
	SiCentos,
	SiDebian,
	SiDeepin,
	SiElementary,
	SiFedora,
	SiFreebsd,
	SiGentoo,
	SiKalilinux,
	SiLinux,
	SiLinuxmint,
	SiMacos,
	SiManjaro,
	SiOpensuse,
	SiOracle,
	SiParrotsecurity,
	SiPopos,
	SiRedhat,
	SiRockylinux,
	SiSlackware,
	SiSolus,
	SiSuse,
	SiTails,
	SiUbuntu,
	SiZorin,
} from "react-icons/si";

/**
 * OS Icon mapping utility
 * Maps operating system types to appropriate react-icons components
 * Now uses specific icons based on actual OS names from /etc/os-release
 */
export const getOSIcon = (osType) => {
	if (!osType) return Monitor;

	const os = osType.toLowerCase();

	// Ubuntu and Ubuntu variants
	if (os.includes("ubuntu")) {
		// For Ubuntu variants, use generic Ubuntu icon as fallback
		return SiUbuntu;
	}

	// Pop!_OS
	if (os.includes("pop") || os.includes("pop!_os")) return SiPopos;

	// Linux Mint
	if (os.includes("mint") || os.includes("linuxmint")) return SiLinuxmint;

	// Elementary OS
	if (os.includes("elementary")) return SiElementary;

	// Debian
	if (os.includes("debian")) return SiDebian;

	// Rocky Linux
	if (os.includes("rocky")) return SiRockylinux;

	// AlmaLinux
	if (os.includes("alma") || os.includes("almalinux")) return SiAlmalinux;

	// CentOS
	if (os.includes("centos")) return SiCentos;

	// Red Hat Enterprise Linux
	if (os.includes("rhel") || os.includes("red hat")) return SiRedhat;

	// Fedora
	if (os.includes("fedora")) return SiFedora;

	// Oracle Linux
	if (os === "ol" || os.includes("oraclelinux") || os.includes("oracle linux"))
		return SiOracle;

	// SUSE distributions
	if (os.includes("opensuse")) return SiOpensuse;
	if (os.includes("suse")) return SiSuse;

	// Arch-based distributions
	if (os.includes("arch")) return SiArchlinux;
	if (os.includes("manjaro")) return SiManjaro;
	if (os.includes("endeavour") || os.includes("endeavouros"))
		return SiArchlinux; // Fallback to Arch
	if (os.includes("garuda")) return SiArchlinux; // Fallback to Arch
	if (os.includes("blackarch")) return SiArchlinux; // Fallback to Arch

	// Other distributions
	if (os.includes("alpine")) return SiAlpinelinux;
	if (os.includes("gentoo")) return SiGentoo;
	if (os.includes("slackware")) return SiSlackware;
	if (os.includes("zorin")) return SiZorin;
	if (os.includes("deepin")) return SiDeepin;
	if (os.includes("solus")) return SiSolus;
	if (os.includes("tails")) return SiTails;
	if (os.includes("parrot")) return SiParrotsecurity;
	if (os.includes("kali")) return SiKalilinux;

	// Generic Linux
	if (os.includes("linux")) return SiLinux;

	// Windows
	if (os.includes("windows")) return DiWindows;

	// macOS
	if (os.includes("mac") || os.includes("darwin")) return SiMacos;

	// FreeBSD
	if (os.includes("freebsd")) return SiFreebsd;

	// Default fallback
	return Monitor;
};

/**
 * OS Color mapping utility
 * Maps operating system types to appropriate colors (react-icons have built-in brand colors)
 */
export const getOSColor = (osType) => {
	if (!osType) return "text-gray-500";

	// react-icons already have the proper brand colors built-in
	// This function is kept for compatibility but returns neutral colors
	return "text-gray-600";
};

/**
 * OS Display name utility
 * Provides clean, formatted OS names for display
 * Updated to handle more distributions from /etc/os-release
 */
export const getOSDisplayName = (osType) => {
	if (!osType) return "Unknown";

	const os = osType.toLowerCase();

	// Ubuntu and variants
	if (os.includes("ubuntu")) {
		if (os.includes("kubuntu")) return "Kubuntu";
		if (os.includes("lubuntu")) return "Lubuntu";
		if (os.includes("xubuntu")) return "Xubuntu";
		if (os.includes("ubuntu mate") || os.includes("ubuntumate"))
			return "Ubuntu MATE";
		if (os.includes("ubuntu budgie") || os.includes("ubuntubudgie"))
			return "Ubuntu Budgie";
		if (os.includes("ubuntu studio") || os.includes("ubuntustudio"))
			return "Ubuntu Studio";
		if (os.includes("ubuntu kylin") || os.includes("ubuntukylin"))
			return "Ubuntu Kylin";
		return "Ubuntu";
	}

	// Pop!_OS
	if (os.includes("pop") || os.includes("pop!_os")) return "Pop!_OS";

	// Linux Mint
	if (os.includes("mint") || os.includes("linuxmint")) return "Linux Mint";

	// Elementary OS
	if (os.includes("elementary")) return "Elementary OS";

	// Debian
	if (os.includes("debian")) return "Debian";

	// Rocky Linux
	if (os.includes("rocky")) return "Rocky Linux";

	// AlmaLinux
	if (os.includes("alma") || os.includes("almalinux")) return "AlmaLinux";

	// CentOS
	if (os.includes("centos")) return "CentOS";

	// Red Hat Enterprise Linux
	if (os.includes("rhel") || os.includes("red hat"))
		return "Red Hat Enterprise Linux";

	// Fedora
	if (os.includes("fedora")) return "Fedora";

	// Oracle Linux
	if (os === "ol" || os.includes("oraclelinux") || os.includes("oracle linux"))
		return "Oracle Linux";

	// SUSE distributions
	if (os.includes("opensuse")) return "openSUSE";
	if (os.includes("suse")) return "SUSE Linux";

	// Arch-based distributions
	if (os.includes("arch")) return "Arch Linux";
	if (os.includes("manjaro")) return "Manjaro";
	if (os.includes("endeavour") || os.includes("endeavouros"))
		return "EndeavourOS";
	if (os.includes("garuda")) return "Garuda Linux";
	if (os.includes("blackarch")) return "BlackArch Linux";

	// Other distributions
	if (os.includes("alpine")) return "Alpine Linux";
	if (os.includes("gentoo")) return "Gentoo";
	if (os.includes("slackware")) return "Slackware";
	if (os.includes("zorin")) return "Zorin OS";
	if (os.includes("deepin")) return "Deepin";
	if (os.includes("solus")) return "Solus";
	if (os.includes("tails")) return "Tails";
	if (os.includes("parrot")) return "Parrot Security";
	if (os.includes("kali")) return "Kali Linux";

	// Generic Linux
	if (os.includes("linux")) return "Linux";

	// Windows
	if (os.includes("windows")) return "Windows";

	// macOS
	if (os.includes("mac") || os.includes("darwin")) return "macOS";

	// FreeBSD
	if (os.includes("freebsd")) return "FreeBSD";

	// Return original if no match
	return osType;
};

/**
 * OS Icon component with proper styling
 */
export const OSIcon = ({ osType, className = "h-4 w-4", showText = false }) => {
	const IconComponent = getOSIcon(osType);
	const displayName = getOSDisplayName(osType);

	if (showText) {
		return (
			<div className="flex items-center gap-2">
				<IconComponent className={className} title={displayName} />
				<span className="text-sm">{displayName}</span>
			</div>
		);
	}

	return <IconComponent className={className} title={displayName} />;
};
