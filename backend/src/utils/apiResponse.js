/**
 * Standardized API Response Utility
 *
 * Provides consistent response formats across all API endpoints.
 * This utility should be used for all new endpoints and gradually adopted
 * in existing endpoints during refactoring.
 *
 * Success Response Format:
 * {
 *   success: true,
 *   data: { ... },
 *   message: "Optional message",
 *   pagination: { ... }  // Optional for paginated responses
 * }
 *
 * Error Response Format:
 * {
 *   success: false,
 *   error: {
 *     code: "ERROR_CODE",
 *     message: "Human readable message",
 *     details: [...] // Optional validation errors array
 *   }
 * }
 */

/**
 * Send a successful response
 * @param {Response} res - Express response object
 * @param {*} data - Response data
 * @param {Object} options - Additional options
 * @param {string} [options.message] - Optional success message
 * @param {Object} [options.pagination] - Optional pagination metadata
 * @param {number} [options.statusCode=200] - HTTP status code
 */
function success(res, data, options = {}) {
	const { message = null, pagination = null, statusCode = 200 } = options;

	const response = {
		success: true,
		data,
	};

	if (message) {
		response.message = message;
	}

	if (pagination) {
		response.pagination = pagination;
	}

	return res.status(statusCode).json(response);
}

/**
 * Send a created response (201)
 * @param {Response} res - Express response object
 * @param {*} data - Created resource data
 * @param {string} [message] - Optional success message
 */
function created(res, data, message = "Resource created successfully") {
	return success(res, data, { message, statusCode: 201 });
}

/**
 * Send an error response
 * @param {Response} res - Express response object
 * @param {string} message - Human readable error message
 * @param {Object} options - Additional options
 * @param {string} [options.code="ERROR"] - Error code for client handling
 * @param {Array} [options.details] - Validation error details
 * @param {number} [options.statusCode=400] - HTTP status code
 */
function error(res, message, options = {}) {
	const { code = "ERROR", details = null, statusCode = 400 } = options;

	const errorObj = {
		code,
		message,
	};

	if (details) {
		errorObj.details = details;
	}

	return res.status(statusCode).json({
		success: false,
		error: errorObj,
	});
}

/**
 * Send a validation error response (400)
 * @param {Response} res - Express response object
 * @param {Array} errors - Array of validation errors from express-validator
 */
function validationError(res, errors) {
	return error(res, "Validation failed", {
		code: "VALIDATION_ERROR",
		details: errors,
		statusCode: 400,
	});
}

/**
 * Send an unauthorized error response (401)
 * @param {Response} res - Express response object
 * @param {string} [message="Unauthorized"] - Error message
 */
function unauthorized(res, message = "Unauthorized") {
	return error(res, message, {
		code: "UNAUTHORIZED",
		statusCode: 401,
	});
}

/**
 * Send a forbidden error response (403)
 * @param {Response} res - Express response object
 * @param {string} [message="Forbidden"] - Error message
 */
function forbidden(res, message = "Forbidden") {
	return error(res, message, {
		code: "FORBIDDEN",
		statusCode: 403,
	});
}

/**
 * Send a not found error response (404)
 * @param {Response} res - Express response object
 * @param {string} [resource="Resource"] - Name of the resource not found
 */
function notFound(res, resource = "Resource") {
	return error(res, `${resource} not found`, {
		code: "NOT_FOUND",
		statusCode: 404,
	});
}

/**
 * Send a conflict error response (409)
 * @param {Response} res - Express response object
 * @param {string} [message="Resource already exists"] - Error message
 */
function conflict(res, message = "Resource already exists") {
	return error(res, message, {
		code: "CONFLICT",
		statusCode: 409,
	});
}

/**
 * Send a server error response (500)
 * @param {Response} res - Express response object
 * @param {string} [message="Internal server error"] - Error message
 */
function serverError(res, message = "Internal server error") {
	return error(res, message, {
		code: "SERVER_ERROR",
		statusCode: 500,
	});
}

/**
 * Send a service unavailable error response (503)
 * @param {Response} res - Express response object
 * @param {string} [message="Service unavailable"] - Error message
 */
function serviceUnavailable(res, message = "Service unavailable") {
	return error(res, message, {
		code: "SERVICE_UNAVAILABLE",
		statusCode: 503,
	});
}

module.exports = {
	success,
	created,
	error,
	validationError,
	unauthorized,
	forbidden,
	notFound,
	conflict,
	serverError,
	serviceUnavailable,
};
