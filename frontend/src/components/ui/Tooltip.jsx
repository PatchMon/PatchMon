import {
	cloneElement,
	isValidElement,
	useCallback,
	useEffect,
	useId,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { createPortal } from "react-dom";

/**
 * Lightweight tooltip used for explainer copy on status pills and similar
 * affordances. Shows on hover (desktop) or focus (keyboard); on touch devices
 * a tap toggles, and a tap elsewhere dismisses. Auto-flips above/below if the
 * preferred side has insufficient room.
 *
 * Renders via React portal to document.body so it escapes any ancestor
 * stacking context (table cells, sticky headers, modals). Z-index 200 sits
 * above the app shell (nav z-[90], sidebar z-[100]).
 *
 * Props:
 *   content   string | ReactNode — body of the tooltip
 *   side      "top" | "bottom" | "left" | "right" (default "top")
 *   className optional extra classes for the bubble
 *   children  the trigger element (must be a single React element)
 */
const Tooltip = ({ content, side = "top", className = "", children }) => {
	const [open, setOpen] = useState(false);
	const [resolvedSide, setResolvedSide] = useState(side);
	const [coords, setCoords] = useState(null);
	const triggerRef = useRef(null);
	const id = useId();

	const computePosition = useCallback(() => {
		if (!triggerRef.current) return;
		const rect = triggerRef.current.getBoundingClientRect();
		const spaceAbove = rect.top;
		const spaceBelow = window.innerHeight - rect.bottom;
		let nextSide = side;
		if (side === "top" && spaceAbove < 56 && spaceBelow > spaceAbove) {
			nextSide = "bottom";
		} else if (
			side === "bottom" &&
			spaceBelow < 56 &&
			spaceAbove > spaceBelow
		) {
			nextSide = "top";
		}
		setResolvedSide(nextSide);
		setCoords({
			top: rect.top,
			bottom: rect.bottom,
			left: rect.left,
			right: rect.right,
			width: rect.width,
			height: rect.height,
		});
	}, [side]);

	// Recompute on open and any scroll/resize while open.
	useLayoutEffect(() => {
		if (!open) return;
		computePosition();
		const handler = () => computePosition();
		window.addEventListener("scroll", handler, true);
		window.addEventListener("resize", handler);
		return () => {
			window.removeEventListener("scroll", handler, true);
			window.removeEventListener("resize", handler);
		};
	}, [open, computePosition]);

	// Tap-elsewhere-to-dismiss for touch devices, and Escape to close
	// (a11y: tooltips must be dismissible without moving focus).
	useEffect(() => {
		if (!open) return;
		const pointerHandler = (e) => {
			if (triggerRef.current && !triggerRef.current.contains(e.target)) {
				setOpen(false);
			}
		};
		const keyHandler = (e) => {
			if (e.key === "Escape") {
				setOpen(false);
			}
		};
		document.addEventListener("pointerdown", pointerHandler, true);
		document.addEventListener("keydown", keyHandler, true);
		return () => {
			document.removeEventListener("pointerdown", pointerHandler, true);
			document.removeEventListener("keydown", keyHandler, true);
		};
	}, [open]);

	if (!isValidElement(children) || !content) return children ?? null;

	const trigger = cloneElement(children, {
		ref: (node) => {
			triggerRef.current = node;
			const childRef = children.ref;
			if (typeof childRef === "function") childRef(node);
			else if (childRef && typeof childRef === "object")
				childRef.current = node;
		},
		"aria-describedby": open ? id : children.props["aria-describedby"],
		onMouseEnter: (e) => {
			setOpen(true);
			children.props.onMouseEnter?.(e);
		},
		onMouseLeave: (e) => {
			setOpen(false);
			children.props.onMouseLeave?.(e);
		},
		onFocus: (e) => {
			setOpen(true);
			children.props.onFocus?.(e);
		},
		onBlur: (e) => {
			setOpen(false);
			children.props.onBlur?.(e);
		},
		onClick: (e) => {
			// Touch / tap support: toggle on click. Mouse users get hover already.
			setOpen((prev) => !prev);
			children.props.onClick?.(e);
		},
	});

	// Compute fixed-position style from the trigger rect + resolved side.
	const bubbleStyle = (() => {
		if (!coords) return { visibility: "hidden" };
		const gap = 8;
		const centerX = coords.left + coords.width / 2;
		const centerY = coords.top + coords.height / 2;
		switch (resolvedSide) {
			case "bottom":
				return {
					position: "fixed",
					top: coords.bottom + gap,
					left: centerX,
					transform: "translateX(-50%)",
				};
			case "left":
				return {
					position: "fixed",
					top: centerY,
					left: coords.left - gap,
					transform: "translate(-100%, -50%)",
				};
			case "right":
				return {
					position: "fixed",
					top: centerY,
					left: coords.right + gap,
					transform: "translateY(-50%)",
				};
			default:
				return {
					position: "fixed",
					top: coords.top - gap,
					left: centerX,
					transform: "translate(-50%, -100%)",
				};
		}
	})();

	// Solid dark bubble + white text in both light and dark mode. The previous
	// dark-mode inversion (light bubble, dark text) clashed with the dark
	// surroundings on the host detail page and was hard to read.
	const arrowClasses = {
		top: "top-full left-1/2 -translate-x-1/2 -mt-1 border-t-secondary-900",
		bottom:
			"bottom-full left-1/2 -translate-x-1/2 -mb-1 border-b-secondary-900",
		left: "left-full top-1/2 -translate-y-1/2 -ml-1 border-l-secondary-900",
		right: "right-full top-1/2 -translate-y-1/2 -mr-1 border-r-secondary-900",
	}[resolvedSide];

	const arrowSide = {
		top: "border-x-4 border-x-transparent border-t-4 border-b-0",
		bottom: "border-x-4 border-x-transparent border-b-4 border-t-0",
		left: "border-y-4 border-y-transparent border-l-4 border-r-0",
		right: "border-y-4 border-y-transparent border-r-4 border-l-0",
	}[resolvedSide];

	const tooltipBubble = open
		? createPortal(
				<span
					role="tooltip"
					id={id}
					style={bubbleStyle}
					className={`pointer-events-none z-[200] whitespace-normal rounded-md bg-secondary-900 px-2 py-1 text-xs font-medium text-white shadow-lg ${className}`}
				>
					<span
						style={{ minWidth: "8rem", maxWidth: "16rem", display: "block" }}
					>
						{content}
					</span>
					<span
						aria-hidden="true"
						className={`absolute h-0 w-0 ${arrowSide} ${arrowClasses}`}
					/>
				</span>,
				document.body,
			)
		: null;

	return (
		<>
			{trigger}
			{tooltipBubble}
		</>
	);
};

export default Tooltip;
export { Tooltip };
