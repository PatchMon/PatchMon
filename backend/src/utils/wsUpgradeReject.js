/**
 * Reject a WebSocket upgrade with a proper HTTP response before closing the socket.
 * Prevents "socket hang up" in proxies (e.g. Vite dev server) when auth fails or path is invalid.
 * Call this instead of socket.destroy() so the client/proxy see a clean response.
 *
 * @param {import("net").Socket} socket - Raw TCP socket from the upgrade event
 * @param {number} status_code - HTTP status (401, 403, 400, 404)
 * @param {string} message - Short error message for the response body
 */
function reject_upgrade(socket, status_code, message) {
	const status_text =
		{
			400: "Bad Request",
			401: "Unauthorized",
			403: "Forbidden",
			404: "Not Found",
		}[status_code] || "Error";

	const body = JSON.stringify({ error: message });
	const response =
		`HTTP/1.1 ${status_code} ${status_text}\r\n` +
		"Content-Type: application/json\r\n" +
		`Content-Length: ${Buffer.byteLength(body, "utf8")}\r\n` +
		"Connection: close\r\n" +
		"\r\n" +
		body;

	if (!socket.destroyed && socket.writable) {
		socket.write(response, "utf8", () => {
			socket.end();
		});
	} else {
		socket.destroy();
	}
}

module.exports = { reject_upgrade };
