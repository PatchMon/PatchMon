import { cleanup } from "@testing-library/react";
import { afterEach, vi } from "vitest";
import "@testing-library/jest-dom";

// Cleanup after each test
afterEach(() => {
	cleanup();
});

// Mock WebSocket - use vi.fn() so tests can assert WebSocket.mock.calls / .mock.results
const WebSocketImpl = class WebSocket {
	constructor(url) {
		this.url = url;
		this.readyState = WebSocketImpl.CONNECTING;
		this.onopen = null;
		this.onclose = null;
		this.onerror = null;
		this.onmessage = null;
		this.send = vi.fn().mockImplementation(function send(_data) {
			if (this.readyState !== WebSocketImpl.OPEN) {
				throw new Error("WebSocket is not open");
			}
		});
		this.close = vi.fn().mockImplementation(function close() {
			this.readyState = WebSocketImpl.CLOSED;
			if (this.onclose) {
				this.onclose({ code: 1000, reason: "Normal closure" });
			}
		});
	}

	// Helper methods for testing
	_simulateOpen() {
		this.readyState = WebSocketImpl.OPEN;
		if (this.onopen) {
			this.onopen();
		}
	}

	_simulateMessage(data) {
		if (this.onmessage) {
			this.onmessage({ data });
		}
	}

	_simulateError(error) {
		if (this.onerror) {
			this.onerror(error);
		}
	}
};

WebSocketImpl.CONNECTING = 0;
WebSocketImpl.OPEN = 1;
WebSocketImpl.CLOSING = 2;
WebSocketImpl.CLOSED = 3;

// Use a regular function so `new WebSocket(url)` works (arrow functions can't be constructors)
// biome-ignore lint/complexity/useArrowFunction: constructor must be a regular function
global.WebSocket = vi.fn().mockImplementation(function (url) {
	return new WebSocketImpl(url);
});
global.WebSocket.CONNECTING = 0;
global.WebSocket.OPEN = 1;
global.WebSocket.CLOSING = 2;
global.WebSocket.CLOSED = 3;
if (typeof window !== "undefined") {
	window.WebSocket = global.WebSocket;
}

// Mock localStorage
const localStorageMock = {
	getItem: vi.fn(),
	setItem: vi.fn(),
	removeItem: vi.fn(),
	clear: vi.fn(),
};
global.localStorage = localStorageMock;

// Mock window.matchMedia
Object.defineProperty(window, "matchMedia", {
	writable: true,
	value: vi.fn().mockImplementation((query) => ({
		matches: false,
		media: query,
		onchange: null,
		addListener: vi.fn(),
		removeListener: vi.fn(),
		addEventListener: vi.fn(),
		removeEventListener: vi.fn(),
		dispatchEvent: vi.fn(),
	})),
});
