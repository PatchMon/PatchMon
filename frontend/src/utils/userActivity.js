// Tracks whether a person has actually used the page, as opposed to the app
// simply making requests. Several screens poll on a timer, so the server cannot
// treat "a request arrived" as "someone is here" without an unattended tab
// keeping its own session alive forever.
//
// Requests report an interaction by sending the X-User-Activity header, which is
// the only thing that slides SESSION_INACTIVITY_TIMEOUT_MINUTES.

const MOUSE_MOVE_THROTTLE_MS = 5000;

let lastInteractionAt = 0;
let lastReportedAt = 0;
let lastMouseMoveAt = 0;

const mark = () => {
	lastInteractionAt = Date.now();
};

// Pointer movement fires continuously; sampling it keeps the listener cheap
// while still counting "reading a screen with a hand on the mouse" as presence.
const markMouseMove = () => {
	const now = Date.now();
	if (now - lastMouseMoveAt < MOUSE_MOVE_THROTTLE_MS) return;
	lastMouseMoveAt = now;
	lastInteractionAt = now;
};

if (typeof window !== "undefined") {
	const opts = { passive: true, capture: true };
	window.addEventListener("mousedown", mark, opts);
	window.addEventListener("keydown", mark, opts);
	window.addEventListener("touchstart", mark, opts);
	window.addEventListener("wheel", mark, opts);
	window.addEventListener("scroll", mark, opts);
	window.addEventListener("mousemove", markMouseMove, opts);
	document.addEventListener("visibilitychange", () => {
		// Returning to the tab is itself a deliberate act.
		if (document.visibilityState === "visible") mark();
	});
}

export const hasPendingInteraction = () => lastInteractionAt > lastReportedAt;

// consumeInteraction reports and clears the pending flag. Callers must only call
// it when they are about to send a request that carries the header.
export const consumeInteraction = () => {
	if (lastInteractionAt <= lastReportedAt) return false;
	lastReportedAt = lastInteractionAt;
	return true;
};
