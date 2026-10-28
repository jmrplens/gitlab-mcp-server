// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package i18n

import (
	"context"
	"slices"
	"strings"
	"testing"
	"unicode"
)

// TestSpanishCatalog_RendersEachCompositionInSpanish pins what a person asked
// in Spanish reads for each way a text is put together: a question, a field
// title, a consent summary whose optional rows are joined by Concat, a
// question whose value is another catalog text (M), a list joined with the
// Spanish conjunction (L), and numbers (N). Each message renders the reviewed
// translation in Spanish and keeps rendering its English source in English,
// so the model, which reads English, is never handed the Spanish.
func TestSpanishCatalog_RendersEachCompositionInSpanish(t *testing.T) {
	for _, tc := range []struct {
		name    string
		msg     Message
		spanish string
		english string
	}{
		{
			name:    "a question",
			msg:     New(InteractiveIssueCreatePromptTitle),
			spanish: "Escribe el título del issue",
			english: "Enter the issue title",
		},
		{
			name:    "a yes or no question",
			msg:     New(InteractiveMrCreatePromptRemoveSourceBranch),
			spanish: "¿Eliminar la rama origen después del merge?",
			english: "Remove source branch after merge?",
		},
		{
			name:    "a destructive confirmation",
			msg:     New(ConfirmDestructiveIndividual, S("tool", "gitlab_branch_delete")),
			spanish: "¿Confirmas gitlab_branch_delete? Esta acción puede ser irreversible.",
			english: "Confirm gitlab_branch_delete? This action may be irreversible.",
		},
		{
			name:    "a field title",
			msg:     New(InteractiveFieldSourceBranch),
			spanish: "Rama origen",
			english: "source_branch",
		},
		{
			name: "a summary with every optional row",
			msg: Concat(
				New(InteractiveIssueCreateSummary, S("project", "group/app"), S("title", "Arreglar el login")),
				New(InteractiveSummaryDescription, S("description", "Primera línea")),
				New(InteractiveSummaryLabels, S("labels", "bug, ui")),
				New(InteractiveIssueCreateSummaryConfidential),
			),
			spanish: "¿Crear el issue en el proyecto group/app?\n\n**Título**: Arreglar el login" +
				"\n**Descripción**: Primera línea\n**Etiquetas**: bug, ui\n**Confidencial**: Sí",
			english: "Create issue in project group/app?\n\n**Title**: Arreglar el login" +
				"\n**Description**: Primera línea\n**Labels**: bug, ui\n**Confidential**: Yes",
		},
		{
			name: "the merge request summary",
			msg: Concat(
				New(InteractiveMrCreateSummary, S("project", "group/app"), S("title", "Login"), S("source", "feature/login"), S("target", "main")),
				New(InteractiveMrCreateSummaryRemoveSourceBranch),
				New(InteractiveMrCreateSummarySquash),
			),
			spanish: "¿Crear el merge request en el proyecto group/app?\n\n**Título**: Login\n**Origen**: feature/login -> **Destino**: main" +
				"\n**Eliminar la rama origen**: Sí\n**Squash de los commits**: Sí",
			english: "Create merge request in project group/app?\n\n**Title**: Login\n**Source**: feature/login -> **Target**: main" +
				"\n**Remove source branch**: Yes\n**Squash commits**: Yes",
		},
		{
			name: "a question whose value is another catalog text",
			msg: New(ConfirmPullMirrorOverwrite,
				S("project", "42"), M("source", New(ConfirmPullMirrorConfiguredSource))),
			spanish: "¿Configurar el proyecto 42 para hacer pull desde su origen de mirror configurado y sobrescribir las ramas divergentes?" +
				" Cada rama de este proyecto que haya divergido de ese repositorio se sustituye por su versión en la próxima sincronización," +
				" y los commits sustituidos no se pueden recuperar mediante la API.",
			english: "Configure project 42 to pull from its configured mirror source and overwrite diverged branches?" +
				" Every branch of this project that has diverged from that repository is replaced by its version on the next sync," +
				" and the replaced commits cannot be recovered through the API.",
		},
		{
			name: "a list joined with the conjunction",
			msg: New(ConfirmWorkItemClear,
				L("losses", New(ConfirmWorkItemAssignees, N("count", 3)), New(ConfirmWorkItemCrmContacts)),
				S("path", "group/app"), N("iid", 42)),
			spanish: "¿Quitar todos los asignados actuales (3) y todos los contactos CRM del work item group/app#42?" +
				" Esto sustituye la lista y no se puede deshacer desde la respuesta.",
			english: "Remove all 3 assignees and every CRM contact from work item group/app#42?" +
				" This replaces the list and cannot be undone from the response.",
		},
		{
			name:    "numbers",
			msg:     New(ProgressPipelineWaitPoll, N("pipeline", 1234), N("attempt", 7)),
			spanish: "Sondeando el pipeline #1234 (intento 7, comprobación de estado)...",
			english: "Polling pipeline #1234 (attempt 7, status check)...",
		},
		{
			name:    "numbers in a byte count",
			msg:     New(ProgressUploadReading, N("read", 65536), N("total", 1048576)),
			spanish: "Leídos 65536 de 1048576 bytes, preparando la subida",
			english: "Read 65536 / 1048576 bytes, preparing the upload",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.msg.In(Spanish); got != tc.spanish {
				t.Errorf("In(es) =\n%q\nwant\n%q", got, tc.spanish)
			}
			if got := tc.msg.English(); got != tc.english {
				t.Errorf("English() =\n%q\nwant\n%q", got, tc.english)
			}
		})
	}
}

// TestSpanishCatalog_IsWhatARequestInSpanishIsServed pins the path a handler
// takes: a context the server's middleware set to Spanish renders the
// Spanish template through T and through Message.Text, and the same message
// still renders English for the model through Message.English.
func TestSpanishCatalog_IsWhatARequestInSpanishIsServed(t *testing.T) {
	ctx := WithLanguage(context.Background(), Spanish)
	const want = "Recopilando los datos del issue..."

	if got := T(ctx, InteractiveIssueCreateProgressDetails).String(); got != want {
		t.Errorf("T(es) = %q, want %q", got, want)
	}
	msg := New(InteractiveIssueCreateProgressDetails)
	if got := msg.Text(ctx).String(); got != want {
		t.Errorf("Text(es) = %q, want %q", got, want)
	}
	if got := msg.English(); got != "Collecting issue details..." {
		t.Errorf("English() under a Spanish request = %q, want the English source", got)
	}
}

// TestSpanishCatalog_FollowsItsGlossary holds the catalog to the glossary its
// translation was written to, which follows what most of the Spanish half of
// the documentation site writes and what its elicitation and progress pages,
// the ones a person reads beside these texts, use: the person is addressed as
// tú (escribe, selecciona), never as usted; the GitLab nouns the site keeps
// in English (issue, merge request, release, pipeline) are not translated;
// and the branch and file terms are rama origen, rama destino, rama
// predeterminada and archivo. The site is not uniform, and writes rama de
// destino and fichero on other pages, so each phrase below is a variant the
// catalog decided against rather than one the site never uses. A phrase is
// matched as whole words, so selecciones (the title of a multiple choice) is
// not taken for seleccione.
func TestSpanishCatalog_FollowsItsGlossary(t *testing.T) {
	rejected := []string{
		"escriba", "seleccione", "introduzca", "ingrese", "indique",
		"incidencia", "solicitud de fusión", "lanzamiento", "canalización",
		"rama fuente", "rama de origen", "rama de destino", "rama por defecto",
		"fichero", "usted",
	}
	for key, template := range spanish {
		t.Run(string(key), func(t *testing.T) {
			words := " " + strings.Join(wordsOf(template), " ") + " "
			var used []string
			for _, phrase := range rejected {
				if strings.Contains(words, " "+phrase+" ") {
					used = append(used, phrase)
				}
			}
			if len(used) != 0 {
				t.Errorf("%s = %q uses %q, which the catalog's glossary decided against", key, template, used)
			}
		})
	}
}

// wordsOf returns the words of a template in lower case, in order, anything
// that is not a letter separating them.
func wordsOf(template string) []string {
	return strings.FieldsFunc(strings.ToLower(template), func(r rune) bool { return !unicode.IsLetter(r) })
}

// TestWordsOf_SplitsOnEverythingButLetters holds the helper the glossary rule
// rests on: accented letters are letters, and punctuation, marks, digits,
// placeholders' braces and line breaks all separate words.
func TestWordsOf_SplitsOnEverythingButLetters(t *testing.T) {
	got := wordsOf("¿Crear {project}?\n\n**Título**: v1 Selecciones, rama-origen")
	want := []string{"crear", "project", "título", "v", "selecciones", "rama", "origen"}
	if !slices.Equal(got, want) {
		t.Errorf("wordsOf() = %q, want %q", got, want)
	}
}
