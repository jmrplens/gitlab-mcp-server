package toolutil

// ActionMetaEntry holds the per-action discovery metadata that domains overlay
// on top of their shared [ActionSpecOptions] defaults: a non-generic Usage
// line, distinctive natural-language Aliases, canonical RelatedActions,
// per-parameter Guidance, and the "Returns: … See also: …" individual-tool
// Description (1:1 audit R-META). Every GitLab domain that carries an action
// metadata table uses this single type instead of a domain-local struct, and
// applies it with [ApplyActionMeta], so the overlay logic lives in one place.
//
// Zero-value fields are skipped by [ApplyActionMeta]; only fields a domain
// actually populates override the option defaults.
type ActionMetaEntry struct {
	// Usage replaces the generic action Usage line when non-empty.
	Usage string
	// Aliases replaces the default tool-name alias list when non-empty.
	Aliases []string
	// Related replaces RelatedActions when non-empty.
	Related []string
	// Guidance replaces ParameterGuidance when non-empty.
	Guidance map[string]ParameterGuidance
	// Description sets the individual-tool description when non-empty.
	Description string
}

// ApplyActionMeta overlays the non-zero fields of meta onto options. Aliases
// and RelatedActions are copied defensively so callers may share a metadata
// table across actions without aliasing the underlying slices. It is a no-op
// for a zero-value entry, so a missing metadata row leaves the option defaults
// untouched.
func ApplyActionMeta(options *ActionSpecOptions, meta ActionMetaEntry) {
	if options == nil {
		return
	}
	if meta.Usage != "" {
		options.Usage = meta.Usage
	}
	if len(meta.Aliases) > 0 {
		options.Aliases = append([]string(nil), meta.Aliases...)
	}
	if len(meta.Related) > 0 {
		options.RelatedActions = append([]string(nil), meta.Related...)
	}
	if len(meta.Guidance) > 0 {
		options.ParameterGuidance = meta.Guidance
	}
	if meta.Description != "" {
		options.IndividualTool.Description = meta.Description
	}
}
