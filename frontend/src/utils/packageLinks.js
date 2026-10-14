export const packageDetailPath = (name, packageIds) => {
	if (!name) return null;
	if (packageIds && typeof packageIds === "object") {
		const id = packageIds[name];
		return id ? `/packages/${encodeURIComponent(id)}` : null;
	}
	return `/packages/${encodeURIComponent(name)}`;
};
