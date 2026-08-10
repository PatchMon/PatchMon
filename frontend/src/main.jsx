import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App.jsx";
import "./index.css";

// Create a client for React Query
//
// Focus refetching is on: v5's focusManager listens for visibilitychange only,
// so this fires when someone actually returns to the tab, not on every click in
// the window. It is the cheapest way to keep a screen current, because it is
// bounded by human tab-switching rather than a timer.
//
// staleTime has to stay well under the time a user is away for that to mean
// anything: both focus and mount refetching only act on queries that are already
// stale, so the previous 5 minutes made both close to no-ops.
//
// Mount refetching, not focus, is the larger multiplier here, since it fires on
// every route change past the stale mark. That is the intent for monitoring
// data, but it makes an expensive aggregate expensive per navigation, so any
// query heavy enough to notice sets a longer staleTime of its own and says why
// (see packageTrends in Dashboard.jsx). A Refresh button still forces those.
//
// None of this touches session lifetime. The inactivity window only slides on
// requests carrying X-User-Activity, which utils/api.js attaches only after a
// real interaction, so background refetching is invisible to the session.
const queryClient = new QueryClient({
	defaultOptions: {
		queries: {
			refetchOnWindowFocus: true,
			retry: 1,
			staleTime: 60 * 1000, // 1 minute
		},
	},
});

ReactDOM.createRoot(document.getElementById("root")).render(
	<StrictMode>
		<BrowserRouter>
			<QueryClientProvider client={queryClient}>
				<App />
			</QueryClientProvider>
		</BrowserRouter>
	</StrictMode>,
);
