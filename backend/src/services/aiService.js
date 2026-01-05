const logger = require("../utils/logger");
const { decrypt } = require("../utils/encryption");

// Provider configurations
const PROVIDERS = {
	openrouter: {
		name: "OpenRouter",
		baseUrl: "https://openrouter.ai/api/v1",
		models: [
			{ id: "anthropic/claude-3.5-sonnet", name: "Claude 3.5 Sonnet" },
			{ id: "anthropic/claude-3-haiku", name: "Claude 3 Haiku" },
			{ id: "openai/gpt-4o", name: "GPT-4o" },
			{ id: "openai/gpt-4o-mini", name: "GPT-4o Mini" },
			{ id: "google/gemini-pro-1.5", name: "Gemini Pro 1.5" },
			{ id: "meta-llama/llama-3.1-70b-instruct", name: "Llama 3.1 70B" },
		],
		defaultModel: "anthropic/claude-3.5-sonnet",
	},
	anthropic: {
		name: "Anthropic Claude",
		baseUrl: "https://api.anthropic.com/v1",
		models: [
			{ id: "claude-sonnet-4-20250514", name: "Claude Sonnet 4" },
			{ id: "claude-3-5-sonnet-20241022", name: "Claude 3.5 Sonnet" },
			{ id: "claude-3-5-haiku-20241022", name: "Claude 3.5 Haiku" },
		],
		defaultModel: "claude-sonnet-4-20250514",
	},
	openai: {
		name: "OpenAI",
		baseUrl: "https://api.openai.com/v1",
		models: [
			{ id: "gpt-4o", name: "GPT-4o" },
			{ id: "gpt-4o-mini", name: "GPT-4o Mini" },
			{ id: "gpt-4-turbo", name: "GPT-4 Turbo" },
		],
		defaultModel: "gpt-4o-mini",
	},
	gemini: {
		name: "Google Gemini",
		baseUrl: "https://generativelanguage.googleapis.com/v1beta",
		models: [
			{ id: "gemini-1.5-pro", name: "Gemini 1.5 Pro" },
			{ id: "gemini-1.5-flash", name: "Gemini 1.5 Flash" },
			{ id: "gemini-2.0-flash-exp", name: "Gemini 2.0 Flash" },
		],
		defaultModel: "gemini-1.5-flash",
	},
};

// System prompts for different use cases
const SYSTEM_PROMPTS = {
	assistant: `You are a helpful terminal assistant integrated into PatchMon, a server management tool.
Your role is to help system administrators understand terminal output, diagnose issues, and suggest solutions.

Guidelines:
- Be concise and direct in your responses
- When explaining errors, provide the likely cause and solution
- Suggest specific commands when appropriate
- Format command suggestions in code blocks
- If you're unsure, say so rather than guessing
- Focus on Linux/Unix system administration topics`,

	completion: `You are a terminal command completion assistant. Given the current command being typed and recent terminal context, suggest the most likely command completion.

Guidelines:
- Only respond with the completion text (what comes after the cursor)
- If multiple completions are possible, choose the most common/useful one
- Keep completions practical and safe
- Do not include explanations, just the completion
- If no good completion exists, respond with an empty string
- Consider the terminal history for context`,
};

/**
 * Get available providers list
 */
function getProviders() {
	return Object.entries(PROVIDERS).map(([id, config]) => ({
		id,
		name: config.name,
		models: config.models,
		defaultModel: config.defaultModel,
	}));
}

/**
 * Call OpenRouter API
 */
async function callOpenRouter(apiKey, model, messages, options = {}) {
	const response = await fetch(`${PROVIDERS.openrouter.baseUrl}/chat/completions`, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
			"Authorization": `Bearer ${apiKey}`,
			"HTTP-Referer": "https://patchmon.app",
			"X-Title": "PatchMon Terminal Assistant",
		},
		body: JSON.stringify({
			model: model || PROVIDERS.openrouter.defaultModel,
			messages,
			max_tokens: options.maxTokens || 1024,
			temperature: options.temperature || 0.7,
			stream: false,
		}),
	});

	if (!response.ok) {
		const error = await response.text();
		throw new Error(`OpenRouter API error: ${response.status} - ${error}`);
	}

	const data = await response.json();
	return data.choices[0]?.message?.content || "";
}

/**
 * Call Anthropic Claude API
 */
async function callAnthropic(apiKey, model, messages, options = {}) {
	// Convert messages format for Anthropic
	const systemMessage = messages.find(m => m.role === "system")?.content || "";
	const conversationMessages = messages.filter(m => m.role !== "system");

	const response = await fetch(`${PROVIDERS.anthropic.baseUrl}/messages`, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
			"x-api-key": apiKey,
			"anthropic-version": "2023-06-01",
		},
		body: JSON.stringify({
			model: model || PROVIDERS.anthropic.defaultModel,
			max_tokens: options.maxTokens || 1024,
			system: systemMessage,
			messages: conversationMessages,
		}),
	});

	if (!response.ok) {
		const error = await response.text();
		throw new Error(`Anthropic API error: ${response.status} - ${error}`);
	}

	const data = await response.json();
	return data.content[0]?.text || "";
}

/**
 * Call OpenAI API
 */
async function callOpenAI(apiKey, model, messages, options = {}) {
	const response = await fetch(`${PROVIDERS.openai.baseUrl}/chat/completions`, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
			"Authorization": `Bearer ${apiKey}`,
		},
		body: JSON.stringify({
			model: model || PROVIDERS.openai.defaultModel,
			messages,
			max_tokens: options.maxTokens || 1024,
			temperature: options.temperature || 0.7,
		}),
	});

	if (!response.ok) {
		const error = await response.text();
		throw new Error(`OpenAI API error: ${response.status} - ${error}`);
	}

	const data = await response.json();
	return data.choices[0]?.message?.content || "";
}

/**
 * Call Google Gemini API
 */
async function callGemini(apiKey, model, messages, options = {}) {
	const modelId = model || PROVIDERS.gemini.defaultModel;

	// Convert messages format for Gemini
	const contents = messages
		.filter(m => m.role !== "system")
		.map(m => ({
			role: m.role === "assistant" ? "model" : "user",
			parts: [{ text: m.content }],
		}));

	// Add system instruction if present
	const systemMessage = messages.find(m => m.role === "system")?.content;

	const response = await fetch(
		`${PROVIDERS.gemini.baseUrl}/models/${modelId}:generateContent?key=${apiKey}`,
		{
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify({
				contents,
				systemInstruction: systemMessage ? { parts: [{ text: systemMessage }] } : undefined,
				generationConfig: {
					maxOutputTokens: options.maxTokens || 1024,
					temperature: options.temperature || 0.7,
				},
			}),
		}
	);

	if (!response.ok) {
		const error = await response.text();
		throw new Error(`Gemini API error: ${response.status} - ${error}`);
	}

	const data = await response.json();
	return data.candidates?.[0]?.content?.parts?.[0]?.text || "";
}

/**
 * Main function to call AI provider
 * @param {Object} settings - AI settings with provider, model, and encrypted API key
 * @param {string} prompt - User prompt
 * @param {Object} options - Additional options
 * @returns {Promise<string>} - AI response
 */
async function callAI(settings, prompt, options = {}) {
	const { ai_provider, ai_model, ai_api_key } = settings;

	if (!ai_api_key) {
		throw new Error("AI API key not configured");
	}

	// Decrypt the API key
	const apiKey = decrypt(ai_api_key);
	if (!apiKey) {
		throw new Error("Failed to decrypt AI API key");
	}

	// Build messages array
	const systemPrompt = options.type === "completion"
		? SYSTEM_PROMPTS.completion
		: SYSTEM_PROMPTS.assistant;

	const messages = [
		{ role: "system", content: systemPrompt },
	];

	// Add context if provided
	if (options.context) {
		messages.push({
			role: "user",
			content: `Recent terminal output:\n\`\`\`\n${options.context}\n\`\`\``,
		});
		messages.push({
			role: "assistant",
			content: "I've noted the terminal context. How can I help?",
		});
	}

	// Add conversation history if provided
	if (options.history && Array.isArray(options.history)) {
		messages.push(...options.history);
	}

	// Add the current prompt
	messages.push({ role: "user", content: prompt });

	// Call the appropriate provider
	const callOptions = {
		maxTokens: options.type === "completion" ? 100 : 1024,
		temperature: options.type === "completion" ? 0.3 : 0.7,
	};

	switch (ai_provider) {
		case "openrouter":
			return callOpenRouter(apiKey, ai_model, messages, callOptions);
		case "anthropic":
			return callAnthropic(apiKey, ai_model, messages, callOptions);
		case "openai":
			return callOpenAI(apiKey, ai_model, messages, callOptions);
		case "gemini":
			return callGemini(apiKey, ai_model, messages, callOptions);
		default:
			throw new Error(`Unknown AI provider: ${ai_provider}`);
	}
}

/**
 * Get command completion suggestion
 * @param {Object} settings - AI settings
 * @param {string} currentInput - Current command being typed
 * @param {string} terminalContext - Recent terminal output for context
 * @returns {Promise<string>} - Suggested completion
 */
async function getCompletion(settings, currentInput, terminalContext = "") {
	if (!currentInput || currentInput.length < 2) {
		return "";
	}

	const prompt = `Current command being typed: "${currentInput}"
Complete this command. Only respond with the remaining text to add, nothing else.`;

	try {
		const completion = await callAI(settings, prompt, {
			type: "completion",
			context: terminalContext,
		});

		// Clean up the response - remove quotes, newlines, etc.
		return completion.trim().replace(/^["']|["']$/g, "");
	} catch (error) {
		logger.debug("AI completion error:", error.message);
		return "";
	}
}

/**
 * Get terminal assistance
 * @param {Object} settings - AI settings
 * @param {string} question - User's question
 * @param {string} terminalContext - Recent terminal output
 * @param {Array} history - Conversation history
 * @returns {Promise<string>} - AI response
 */
async function getAssistance(settings, question, terminalContext = "", history = []) {
	return callAI(settings, question, {
		type: "assistant",
		context: terminalContext,
		history,
	});
}

module.exports = {
	getProviders,
	callAI,
	getCompletion,
	getAssistance,
	PROVIDERS,
};
