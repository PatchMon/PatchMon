import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";
import "@testing-library/jest-dom";

// Cleanup after each test
afterEach(() => {
	cleanup();
});

// Mock WebSocket
global.WebSocket = class WebSocket {
	constructor(url) {
		this.url = url;
		this.readyState = WebSocket.CONNECTING;
		this.onopen = null;
		this.onclose = null;
		this.onerror = null;
		this.onmessage = null;
	}

	send(_data) {
		if (this.readyState !== WebSocket.OPEN) {
			throw new Error("WebSocket is not open");
		}
	}

	close() {
		this.readyState = WebSocket.CLOSED;
		if (this.onclose) {
			this.onclose({ code: 1000, reason: "Normal closure" });
		}
	}

	// Helper methods for testing
	_simulateOpen() {
		this.readyState = WebSocket.OPEN;
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

WebSocket.CONNECTING = 0;
WebSocket.OPEN = 1;
WebSocket.CLOSING = 2;
WebSocket.CLOSED = 3;

import { vi } from "vitest";

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
		addListener: jest.fn(),
		removeListener: jest.fn(),
		addEventListener: jest.fn(),
		removeEventListener: jest.fn(),
		dispatchEvent: jest.fn(),
	})),
});
