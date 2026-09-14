import axios from "axios";

const API_BASE_URL = import.meta.env.VITE_API_URL || "/api/v1";

// Single-flight: a burst of requests hitting the same expired access token must
// produce one refresh, not one per request. Shared by the axios instance and the
// fetch wrapper below so the two paths cannot race each other.
let refreshInFlight = null;

// Bare axios on purpose - routing this through the shared `api` instance would
// re-enter its response interceptor and recurse.
export const requestTokenRefresh = () => {
	if (!refreshInFlight) {
		refreshInFlight = axios
			.post(`${API_BASE_URL}/auth/refresh`, null, { withCredentials: true })
			.finally(() => {
				refreshInFlight = null;
			});
	}
	return refreshInFlight;
};

// Drop-in `fetch` for authenticated endpoints. AuthContext talks to the API with
// fetch rather than the axios instance, so without this a page load after the
// access token had expired would send a perfectly live session to the login
// screen. Retries once; anything still 401 is a genuinely dead session and is
// returned to the caller to handle.
export const fetchWithSessionRefresh = async (url, options) => {
	const response = await fetch(url, options);
	if (response.status !== 401) return response;
	try {
		await requestTokenRefresh();
	} catch {
		return response;
	}
	return fetch(url, options);
};
