// The translation keys src/content/i18n/*.json adds on top of Starlight's own.
//
// Starlight types Astro.locals.t from StarlightApp.I18n (see its global.d.ts),
// and since 0.42 that typing is strict: a key it does not know is refused
// rather than accepted as a string. Declaring ours here is what lets a
// component call t("gm.fact.exposes") with the key checked at compile time;
// a key added to the JSON files is added here too, or the call will not type.
declare namespace StarlightApp {
	interface I18n {
		"gm.fact.exposes": string;
		"gm.fact.requires": string;
		"gm.fact.tier": string;
		"gm.fact.notCovered": string;
		"gm.fact.symptom": string;
		"gm.fact.meaning": string;
		"gm.fact.fixes": string;
	}
}
