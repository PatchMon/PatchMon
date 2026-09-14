// Placement helpers for popovers that escape a clipping ancestor by using
// `position: fixed`. Fixed coordinates are viewport relative, so the popover
// has to be clamped to the viewport as well: anchoring it below its trigger
// unconditionally puts it out of reach for any trigger near the window bottom,
// and page scroll offsets must not be added at all.

const GAP = 4;
const MARGIN = 8;
const MIN_HEIGHT = 120;

// Returns the viewport `top` for a popover anchored to `rect`, plus the
// `maxHeight` it may occupy there. It opens downwards whenever that side fits
// or is the roomier one, and flips above otherwise. `maxHeight` is always the
// real space on the chosen side, so an inaccurate `desiredHeight` can only
// change which way it opens, never leave content unreachable.
export const anchorVertically = (rect, desiredHeight) => {
	const spaceBelow = window.innerHeight - rect.bottom - GAP - MARGIN;
	const spaceAbove = rect.top - GAP - MARGIN;

	if (spaceBelow >= desiredHeight || spaceBelow >= spaceAbove) {
		return {
			top: rect.bottom + GAP,
			maxHeight: Math.max(spaceBelow, MIN_HEIGHT),
		};
	}

	const height = Math.min(desiredHeight, Math.max(spaceAbove, MIN_HEIGHT));
	return {
		top: Math.max(MARGIN, rect.top - GAP - height),
		maxHeight: Math.max(spaceAbove, MIN_HEIGHT),
	};
};

// Keeps a popover of `width` inside the window when its trigger sits close to
// the right edge.
export const clampHorizontally = (left, width) =>
	Math.max(MARGIN, Math.min(left, window.innerWidth - width - MARGIN));
