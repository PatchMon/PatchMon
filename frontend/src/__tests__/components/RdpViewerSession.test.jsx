import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const guac = vi.hoisted(() => ({
	client: null,
	clients: [],
	keyboards: [],
	display: null,
}));

vi.mock("guacamole-common-js", async () => {
	const actual = (await vi.importActual("guacamole-common-js")).default;
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
		this.sentClipboard = [];
		guac.client = this;
		guac.clients.push(this);
	}
	function Keyboard(element) {
		const keyboard = new actual.Keyboard(element);
		guac.keyboards.push(keyboard);
		return keyboard;
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
const KEYSYM_Z = 0x7a;
const KEYSYM_META = 0xffe7;

const KEYS = {
	Control: { code: "ControlLeft", keyCode: 17 },
	Meta: { code: "MetaLeft", keyCode: 91 },
	v: { code: "KeyV", keyCode: 86 },
	z: { code: "KeyZ", keyCode: 90 },
};

let resizeCallback;
const originalPlatform = Object.getOwnPropertyDescriptor(
	Object.getPrototypeOf(navigator),
	"platform",
);

const setPlatform = (value) =>
	Object.defineProperty(navigator, "platform", { value, configurable: true });

const viewerElement = () => document.querySelector("[tabindex='0']");

const key = (type, name, mods = {}) => {
	const { code, keyCode } = KEYS[name];
	const fire = type === "down" ? fireEvent.keyDown : fireEvent.keyUp;
	act(() => {
		fire(viewerElement(), {
			key: name,
			code,
			keyCode,
			which: keyCode,
			...mods,
		});
	});
};

const paste = (text) => {
	const event = new Event("paste", { bubbles: true });
	event.clipboardData = { getData: () => text };
	act(() => {
		viewerElement().dispatchEvent(event);
	});
};

const keyCalls = () => guac.client.sendKeyEvent.mock.calls;

const startConnect = async () => {
	await act(async () => {
		fireEvent.click(screen.getByRole("button", { name: "Connect" }));
	});
};

const markConnected = () =>
	act(() => {
		guac.client.onstatechange(Guacamole.Client.State.CONNECTED);
	});

const connectViewer = async () => {
	render(<RdpViewer host={{ id: "h1", hostname: "win" }} isOpen />);
	await startConnect();
	markConnected();
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
	guac.clients = [];
	guac.keyboards = [];
	setPlatform("Win32");
});

afterEach(() => {
	delete navigator.platform;
	if (originalPlatform) {
		Object.defineProperty(navigator, "platform", originalPlatform);
	}
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
		expect(guac.client.sendSize).not.toHaveBeenCalled();
	});

	it("sends the chosen keyboard layout with the ticket request", async () => {
		render(<RdpViewer host={{ id: "h1", hostname: "win" }} isOpen />);
		fireEvent.change(screen.getByLabelText("Keyboard layout"), {
			target: { value: "en-gb-qwerty" },
		});
		await startConnect();
		expect(rdpAPI.createTicket).toHaveBeenCalledWith(
			expect.objectContaining({ keyboardLayout: "en-gb-qwerty" }),
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

	it("sends a resize made while connecting once the session is connected", async () => {
		render(<RdpViewer host={{ id: "h1", hostname: "win" }} isOpen />);
		await startConnect();
		vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockReturnValue(1600);
		vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockReturnValue(900);
		act(() => resizeCallback());
		act(() => vi.advanceTimersByTime(1000));
		expect(guac.client.sendSize).not.toHaveBeenCalled();
		markConnected();
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

	it("sends the browser clipboard before Ctrl+V reaches the remote", async () => {
		await connectViewer();
		key("down", "Control", { ctrlKey: true });
		key("down", "v", { ctrlKey: true });
		expect(keyCalls()).not.toContainEqual([1, KEYSYM_V]);
		paste("from browser");
		key("up", "v", { ctrlKey: true });
		key("up", "Control");
		expect(guac.client.sentClipboard).toEqual(["from browser"]);
		expect(keyCalls()).toEqual([
			[1, KEYSYM_CTRL],
			[1, KEYSYM_V],
			[0, KEYSYM_V],
			[0, KEYSYM_CTRL],
		]);
	});

	it("sends macOS Cmd+V as Ctrl+V even though V is released before the paste event", async () => {
		setPlatform("MacIntel");
		await connectViewer();
		key("down", "Meta", { metaKey: true });
		key("down", "v", { metaKey: true });
		key("up", "v", { metaKey: true });
		expect(keyCalls()).toEqual([[1, KEYSYM_META]]);
		paste("from mac");
		expect(guac.client.sentClipboard).toEqual(["from mac"]);
		expect(keyCalls()).toEqual([
			[1, KEYSYM_META],
			[1, KEYSYM_CTRL],
			[0, KEYSYM_META],
			[1, KEYSYM_V],
			[0, KEYSYM_V],
			[0, KEYSYM_CTRL],
		]);
	});

	it("re-presses Cmd when another key follows a translated paste", async () => {
		setPlatform("MacIntel");
		await connectViewer();
		key("down", "Meta", { metaKey: true });
		key("down", "v", { metaKey: true });
		key("up", "v", { metaKey: true });
		paste("from mac");
		guac.client.sendKeyEvent.mockClear();
		key("down", "z", { metaKey: true });
		expect(keyCalls().slice(0, 2)).toEqual([
			[1, KEYSYM_META],
			[1, KEYSYM_Z],
		]);
	});

	it("does not send a stray Cmd release after a translated paste", async () => {
		setPlatform("MacIntel");
		await connectViewer();
		key("down", "Meta", { metaKey: true });
		key("down", "v", { metaKey: true });
		key("up", "v", { metaKey: true });
		paste("from mac");
		guac.client.sendKeyEvent.mockClear();
		key("up", "Meta");
		expect(keyCalls()).toEqual([]);
	});

	it("leaves Win+V alone outside macOS", async () => {
		await connectViewer();
		key("down", "Meta", { metaKey: true });
		key("down", "v", { metaKey: true });
		expect(keyCalls()).toEqual([
			[1, KEYSYM_META],
			[1, KEYSYM_V],
			[0, KEYSYM_V],
		]);
	});

	it("still sends Ctrl+V when the browser fires no paste event", async () => {
		await connectViewer();
		key("down", "Control", { ctrlKey: true });
		key("down", "v", { ctrlKey: true });
		act(() => vi.advanceTimersByTime(500));
		expect(keyCalls()).toContainEqual([1, KEYSYM_V]);
	});

	it("keeps Ctrl+V a Ctrl+V on macOS when no paste event fires and Ctrl is already up", async () => {
		setPlatform("MacIntel");
		await connectViewer();
		key("down", "Control", { ctrlKey: true });
		key("down", "v", { ctrlKey: true });
		key("up", "v", { ctrlKey: true });
		key("up", "Control");
		act(() => vi.advanceTimersByTime(500));
		expect(keyCalls()).toEqual([
			[1, KEYSYM_CTRL],
			[0, KEYSYM_CTRL],
			[1, KEYSYM_CTRL],
			[1, KEYSYM_V],
			[0, KEYSYM_V],
			[0, KEYSYM_CTRL],
		]);
	});

	it("pastes through the new session after a reconnect", async () => {
		await connectViewer();
		fireEvent.click(screen.getByRole("button", { name: /disconnect/i }));
		await startConnect();
		markConnected();
		expect(guac.clients).toHaveLength(2);
		const [first, second] = guac.clients;
		key("down", "Control", { ctrlKey: true });
		key("down", "v", { ctrlKey: true });
		paste("after reconnect");
		expect(first.sendKeyEvent).not.toHaveBeenCalled();
		expect(first.sentClipboard).toEqual([]);
		expect(second.sentClipboard).toEqual(["after reconnect"]);
		expect(second.sendKeyEvent.mock.calls).toEqual([
			[1, KEYSYM_CTRL],
			[1, KEYSYM_V],
		]);
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
