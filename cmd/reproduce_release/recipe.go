package main

import (
	"fmt"
	"slices"
	"strings"
	"text/template"
)

// fields are the template fields this command evaluates, named as GoReleaser
// names them. A template that names any other field, or any function,
// GoReleaser's own included, fails to render and stops the run, since a
// value guessed for it would make a build the release never ran.
type fields struct {
	// Version is the tag without its leading v, which is what GoReleaser
	// renders for a tag and what the snapshot template of this repository
	// renders in a rehearsal.
	Version string
	// Tag is the release tag.
	Tag string
	// Commit and FullCommit are the commit rebuilt; ShortCommit is git's
	// abbreviation of it, whose length depends on the clone.
	Commit, FullCommit, ShortCommit string
	// Binary, Os and Arch are what an archive's name is rendered from.
	Binary, Os, Arch string
}

// render evaluates one GoReleaser template with f.
func render(text string, f fields) (string, error) {
	tmpl, err := template.New("goreleaser").Option("missingkey=error").Parse(text)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	if execErr := tmpl.Execute(&out, f); execErr != nil {
		return "", execErr
	}
	return out.String(), nil
}

// renderEach evaluates a list of templates the way GoReleaser does, leaving
// out an entry that renders empty.
func renderEach(texts []string, f fields) ([]string, error) {
	out := make([]string, 0, len(texts))
	for _, text := range texts {
		rendered, err := render(text, f)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", text, err)
		}
		if rendered != "" {
			out = append(out, rendered)
		}
	}
	return out, nil
}

// recipe is everything one binary is built from: the go command line, the
// environment the build adds, where GoReleaser writes the file and the name
// the release publishes it under.
type recipe struct {
	build  string
	target target
	// asset is the name the release publishes the binary under, which the
	// checksum file lists it by.
	asset string
	// dir and file are where GoReleaser writes the binary inside dist/. The
	// file's own name is kept as GoReleaser gives it, so nothing the linker
	// derives from the output's name can differ.
	dir, file string
	// flags are the go build arguments before -o: the entry's flags and its
	// -ldflags, in GoReleaser's order.
	flags []string
	main  string
	env   []string
}

// arguments is the go command line that builds the recipe into out.
func (r recipe) arguments(out string) []string {
	return slices.Concat([]string{"build"}, r.flags, []string{"-o", out, r.main})
}

// fatRecipe is a universal darwin binary: the darwin recipes it joins, where
// GoReleaser writes it and the name the release publishes it under.
type fatRecipe struct {
	asset, dir, file string
	slices           []recipe
}

// recipes renders every binary the release publishes: one recipe per target
// of every build, and one per universal binary.
func (r release) recipes(f fields) ([]recipe, []fatRecipe, error) {
	var thin []recipe
	for _, b := range r.builds {
		made, err := b.recipes(f, r.archive)
		if err != nil {
			return nil, nil, err
		}
		thin = append(thin, made...)
	}
	fat := make([]fatRecipe, 0, len(r.universals))
	for _, u := range r.universals {
		made, err := u.recipe(f, r.archive, thin)
		if err != nil {
			return nil, nil, err
		}
		fat = append(fat, made)
	}
	return thin, fat, nil
}

// recipes renders the build for each of its targets.
func (b build) recipes(f fields, archive string) ([]recipe, error) {
	binary, err := render(b.binary, f)
	if err != nil {
		return nil, fmt.Errorf("build %s: binary: %w", b.id, err)
	}
	made := make([]recipe, 0, len(b.targets))
	for _, t := range b.targets {
		rc, recipeErr := b.recipe(t, f, binary, archive)
		if recipeErr != nil {
			return nil, fmt.Errorf("build %s for %s: %w", b.id, t, recipeErr)
		}
		made = append(made, rc)
	}
	return made, nil
}

// recipe renders the build for one target, whose binary is called binary.
func (b build) recipe(t target, f fields, binary, archive string) (recipe, error) {
	f.Binary, f.Os, f.Arch = binary, t.goos, t.goarch
	flags, err := renderEach(b.flags, f)
	if err != nil {
		return recipe{}, fmt.Errorf("flags: %w", err)
	}
	env, err := renderEach(b.env, f)
	if err != nil {
		return recipe{}, fmt.Errorf("env: %w", err)
	}
	linker, err := renderEach(b.linkerFlagsFor(t), f)
	if err != nil {
		return recipe{}, fmt.Errorf("ldflags: %w", err)
	}
	asset, err := render(archive, f)
	if err != nil {
		return recipe{}, fmt.Errorf("archive name_template: %w", err)
	}
	// GoReleaser passes the ldflags as one argument, joined by spaces, and
	// none at all when they render empty.
	if len(linker) > 0 {
		flags = append(flags, "-ldflags="+strings.Join(linker, " "))
	}
	return recipe{
		build: b.id, target: t, asset: asset + t.ext(), dir: t.distDir(b.id), file: binary + t.ext(),
		flags: flags, main: b.main, env: env,
	}, nil
}

// linkerFlagsFor is the ldflags a target is built with: those of the first
// override that matches it, when it sets any, and the build's own otherwise,
// which is how GoReleaser merges an override (withOverrides).
func (b build) linkerFlagsFor(t target) []string {
	for _, o := range b.overrides {
		if !o.matches(t) {
			continue
		}
		if len(o.ldflags) > 0 {
			return o.ldflags
		}
		break
	}
	return b.ldflags
}

// recipe renders the universal binary from the darwin recipes of the builds
// it joins.
func (u universal) recipe(f fields, archive string, thin []recipe) (fatRecipe, error) {
	var joined []recipe
	for _, rc := range thin {
		if rc.target.goos == "darwin" && slices.Contains(u.ids, rc.build) {
			joined = append(joined, rc)
		}
	}
	if len(joined) == 0 {
		return fatRecipe{}, fmt.Errorf("universal binary %s joins no darwin binary", u.id)
	}
	name, err := render(u.name, f)
	if err != nil {
		return fatRecipe{}, fmt.Errorf("universal binary %s: name_template: %w", u.id, err)
	}
	// GoReleaser renders the archive name of a universal binary with the
	// Binary of the first darwin binary it joins, and darwin/all.
	f.Binary, f.Os, f.Arch = joined[0].file, "darwin", "all"
	asset, err := render(archive, f)
	if err != nil {
		return fatRecipe{}, fmt.Errorf("universal binary %s: archive name_template: %w", u.id, err)
	}
	return fatRecipe{asset: asset, dir: u.id + "_darwin_all", file: name, slices: joined}, nil
}
