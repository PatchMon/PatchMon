import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const guac = vi.hoisted(() => ({
	client: null,
	keyboard: null,
	display: null,
}));

vi.mock("guacamole-common-js", () => {
	function WebSocketTunnel() {}
	WebSocketTunnel.prototype.disconnect = () => {};
	function Client() {
		const element = document.createElement("div");
		guac.display = {
			getElement: () => element,
			getWidth: () => 1280,
			getHeight: () => 720,
			scale: vi.fn(),
			moveCursor: vi.fn(),
			onresize: null,
		};
		this.getDisplay = () => guac.display;
		this.connect = vi.fn();
		this.disconnect = vi.fn();
		this.sendKeyEvent = vi.fn();
		this.sendMouseState = vi.fn();
		this.sendSize = vi.fn();
		this.createClipboardStream = vi.fn(() => ({}));
		guac.client = this;
	}
	function Keyboard() {
		guac.keyboard = this;
	}
	function Mouse() {}
	function StringWriter() {
		this.sendText = vi.fn((t) => guac.client.sentClipboard.push(t));
		this.sendEnd = vi.fn();
	}
	function StringReader(stream) {
		stream.reader = this;
	}
	return {
		default: {
			WebSocketTunnel,
			Client,
			Keyboard,
			Mouse,
			StringWriter,
			StringReader,
			Tunnel: { State: { CLOSED: 2 } },
		},
	};
});

vi.mock("../../utils/api", () => ({
	rdpAPI: {
		createTicket: vi.fn(() =>
			Promise.resolve({
				data: {
					ticket: "t",
					websocketTunnelUrl: "/api/v1/rdp/websocket-tunnel",
					width: 1000,
					height: 600,
				},
			}),
		),
	},
}));

import Guacamole from "guacamole-common-js";
import RdpViewer from "../../components/RdpViewer";
import { rdpAPI } from "../../utils/api";

Guacamole.Client.State = { CONNECTING: 1, CONNECTED: 3, DISCONNECTED: 5 };

const KEYSYM_CTRL = 0xffe3;
const KEYSYM_V = 0x76;
const KEYSYM_META = 0xffe7;

let resizeCallback;

const connectViewer = async () => {
	render(<RdpViewer host={{ id: "h1", hostname: "win" }} isOpen />);
	await act(async () => {
		fireEvent.click(screen.getByRole("button", { name: /connect/i }));
	});
	guac.client.sentClipboard = [];
	act(() => {
		guac.client.onstatechange(Guacamole.Client.State.CONNECTED);
	});
};

beforeEach(() => {
	vi.useFakeTimers({ shouldAdvanceTime: true });
	resizeCallback = null;
	globalThis.ResizeObserver = class {
		constructor(cb) {
			resizeCallback = cb;
		}
		observe() {}
		disconnect() {}
	};
	vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockReturnValue(1001);
	vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockReturnValue(601);
});

afterEach(() => {
	vi.useRealTimers();
	vi.restoreAllMocks();
	rdpAPI.createTicket.mockClear();
});

describe("RdpViewer session behaviour", () => {
	it("requests the display size at connect", async () => {
		await connectViewer();
		expect(rdpAPI.createTicket).toHaveBeenCalledWith(
			expect.objectContaining({ width: 1000, height: 600 }),
		);
	});

	it("sends the new container size to the remote session after a resize", async () => {
		await connectViewer();
		vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockReturnValue(1600);
		vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockReturnValue(900);
		act(() => resizeCallback());
		act(() => resizeCallback());
		expect(guac.client.sendSize).not.toHaveBeenCalled();
		act(() => vi.advanceTimersByTime(1000));
		expect(guac.client.sendSize).toHaveBeenCalledTimes(1);
		expect(guac.client.sendSize).toHaveBeenCalledWith(1600, 900);
	});

	it("does not resize the remote session while the viewer is hidden", async () => {
		await connectViewer();
		vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockReturnValue(0);
		vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockReturnValue(0);
		act(() => resizeCallback());
		act(() => vi.advanceTimersByTime(1000));
		expect(guac.client.sendSize).not.toHaveBeenCalled();
	});

	it("lets Ctrl+V reach the browser and sends the browser clipboard before the key", async () => {
		await connectViewer();
		const kb = guac.keyboard;
		kb.onkeydown(KEYSYM_CTRL);
		expect(kb.onkeydown(KEYSYM_V)).toBe(true);
		expect(guac.client.sendKeyEvent).not.toHaveBeenCalledWith(1, KEYSYM_V);

		const paste = new Event("paste", { bubbles: true });
		paste.clipboardData = { getData: () => "from browser" };
		act(() => {
			document.activeElement.dispatchEvent(paste);
		});
		expect(guac.client.sentClipboard).toEqual(["from browser"]);
		expect(guac.client.sendKeyEvent).toHaveBeenLastCalledWith(1, KEYSYM_V);
	});

	it("sends macOS Cmd+V to Windows as Ctrl+V after syncing the clipboard", async () => {
		await connectViewer();
		const kb = guac.keyboard;
		kb.onkeydown(KEYSYM_META);
		expect(kb.onkeydown(KEYSYM_V)).toBe(true);
		const paste = new Event("paste", { bubbles: true });
		paste.clipboardData = { getData: () => "from mac" };
		act(() => {
			document.activeElement.dispatchEvent(paste);
		});
		kb.onkeyup(KEYSYM_V);
		expect(guac.client.sentClipboard).toEqual(["from mac"]);
		expect(guac.client.sendKeyEvent.mock.calls).toEqual([
			[1, KEYSYM_META],
			[1, KEYSYM_CTRL],
			[0, KEYSYM_META],
			[1, KEYSYM_V],
			[0, KEYSYM_V],
			[0, KEYSYM_CTRL],
		]);
	});

	it("still sends Ctrl+V when the browser fires no paste event", async () => {
		await connectViewer();
		const kb = guac.keyboard;
		kb.onkeydown(KEYSYM_CTRL);
		kb.onkeydown(KEYSYM_V);
		act(() => vi.advanceTimersByTime(500));
		expect(guac.client.sendKeyEvent).toHaveBeenLastCalledWith(1, KEYSYM_V);
	});

	it("shows the remote clipboard when the browser clipboard API is unavailable", async () => {
		const original = navigator.clipboard;
		Object.defineProperty(navigator, "clipboard", {
			value: undefined,
			configurable: true,
		});
		try {
			await connectViewer();
			const stream = {};
			act(() => {
				guac.client.onclipboard(stream, "text/plain");
				stream.reader.ontext("copied on ");
				stream.reader.ontext("Windows");
				stream.reader.onend();
			});
			expect(screen.getByLabelText(/^remote clipboard/i)).toHaveValue(
				"copied on Windows",
			);
		} finally {
			Object.defineProperty(navigator, "clipboard", {
				value: original,
				configurable: true,
			});
		}
	});
});
