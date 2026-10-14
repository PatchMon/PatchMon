export const RDP_KEYBOARD_LAYOUTS = [
	{ value: "en-gb-qwerty", label: "English (UK)" },
	{ value: "en-us-qwerty", label: "English (US)" },
	{ value: "cs-cz-qwertz", label: "Czech" },
	{ value: "da-dk-qwerty", label: "Danish" },
	{ value: "de-de-qwertz", label: "German" },
	{ value: "de-ch-qwertz", label: "German (Switzerland)" },
	{ value: "es-es-qwerty", label: "Spanish" },
	{ value: "es-latam-qwerty", label: "Spanish (Latin America)" },
	{ value: "fr-fr-azerty", label: "French" },
	{ value: "fr-be-azerty", label: "French (Belgium)" },
	{ value: "fr-ca-qwerty", label: "French (Canada)" },
	{ value: "fr-ch-qwertz", label: "French (Switzerland)" },
	{ value: "hu-hu-qwertz", label: "Hungarian" },
	{ value: "it-it-qwerty", label: "Italian" },
	{ value: "ja-jp-qwerty", label: "Japanese" },
	{ value: "no-no-qwerty", label: "Norwegian" },
	{ value: "pl-pl-qwerty", label: "Polish" },
	{ value: "pt-pt-qwerty", label: "Portuguese" },
	{ value: "pt-br-qwerty", label: "Portuguese (Brazil)" },
	{ value: "ro-ro-qwerty", label: "Romanian" },
	{ value: "sv-se-qwerty", label: "Swedish" },
	{ value: "tr-tr-qwerty", label: "Turkish" },
];

const SUPPORTED = new Set(RDP_KEYBOARD_LAYOUTS.map((l) => l.value));

const BY_LOCALE = {
	"en-gb": "en-gb-qwerty",
	"en-ie": "en-gb-qwerty",
	"de-ch": "de-ch-qwertz",
	"es-es": "es-es-qwerty",
	"fr-be": "fr-be-azerty",
	"fr-ca": "fr-ca-qwerty",
	"fr-ch": "fr-ch-qwertz",
	"pt-br": "pt-br-qwerty",
};

const BY_LANGUAGE = {
	en: "en-us-qwerty",
	cs: "cs-cz-qwertz",
	da: "da-dk-qwerty",
	de: "de-de-qwertz",
	es: "es-latam-qwerty",
	fr: "fr-fr-azerty",
	hu: "hu-hu-qwertz",
	it: "it-it-qwerty",
	ja: "ja-jp-qwerty",
	nb: "no-no-qwerty",
	nn: "no-no-qwerty",
	no: "no-no-qwerty",
	pl: "pl-pl-qwerty",
	pt: "pt-pt-qwerty",
	ro: "ro-ro-qwerty",
	sv: "sv-se-qwerty",
	tr: "tr-tr-qwerty",
};

export const defaultRdpKeyboardLayout = (languages = []) => {
	for (const tag of languages) {
		const lower = (tag || "").toLowerCase();
		const [language, region] = lower.split("-");
		if (region && BY_LOCALE[`${language}-${region}`]) {
			return BY_LOCALE[`${language}-${region}`];
		}
		if (BY_LANGUAGE[language]) return BY_LANGUAGE[language];
	}
	return "en-us-qwerty";
};

const storageKey = (hostId) => `rdp_keyboard_layout_${hostId}`;

export const loadRdpKeyboardLayout = (hostId, languages) => {
	try {
		const saved = localStorage.getItem(storageKey(hostId));
		if (SUPPORTED.has(saved)) return saved;
	} catch {
		// localStorage might be disabled
	}
	return defaultRdpKeyboardLayout(languages);
};

export const saveRdpKeyboardLayout = (hostId, layout) => {
	try {
		localStorage.setItem(storageKey(hostId), layout);
	} catch {
		// localStorage might be full or disabled
	}
};
