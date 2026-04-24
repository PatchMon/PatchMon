export const isRenderableAvatarSrc = (value) => {
	if (!value || typeof value !== "string") {
		return false;
	}

	const normalized = value.trim();
	if (!normalized) {
		return false;
	}
	if (normalized.toLowerCase().startsWith("data:image/")) {
		return true;
	}

	try {
		const parsed = new URL(normalized);
		return parsed.protocol === "http:" || parsed.protocol === "https:";
	} catch {
		return false;
	}
};
