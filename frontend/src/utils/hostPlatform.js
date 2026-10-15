const platformOf = (host) =>
	(host?.os_type || host?.expected_platform || "").toLowerCase();

export const hostIsWindows = (host) => platformOf(host).includes("windows");

export const hostIsFreeBSD = (host) =>
	(host?.package_manager || "").toLowerCase() === "pkg" ||
	platformOf(host).includes("freebsd");

export const showsSELinuxStatus = (host) =>
	Boolean(host?.selinux_status) && !hostIsWindows(host) && !hostIsFreeBSD(host);
