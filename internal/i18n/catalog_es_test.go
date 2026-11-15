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
//
//nolint:maintidx // one table of data: its length is the number of compositions it renders.
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
				" Cada rama que trae el mirror y que haya divergido de ese repositorio se sustituye por su versión en la próxima sincronización," +
				" y los commits sustituidos no se pueden recuperar mediante la API.",
			english: "Configure project 42 to pull from its configured mirror source and overwrite diverged branches?" +
				" Every branch the mirror pulls that has diverged from that repository is replaced by its version on the next sync," +
				" and the replaced commits cannot be recovered through the API.",
		},
		{
			name: "the same question for a mirror the call leaves off",
			msg: New(ConfirmPullMirrorOverwriteWhileOff,
				S("project", "42"), M("source", New(ConfirmPullMirrorConfiguredSource))),
			spanish: "¿Dejar desactivado el mirror de pull del proyecto 42 y configurarlo para sobrescribir las ramas divergentes" +
				" desde su origen de mirror configurado cuando se active? Desde la primera sincronización posterior," +
				" cada rama que trae el mirror y que haya divergido de ese repositorio se sustituye por su versión," +
				" y los commits sustituidos no se pueden recuperar mediante la API.",
			english: "Leave the pull mirror of project 42 off and set it to overwrite diverged branches from its configured mirror source" +
				" once it is turned on? From the first sync after that, every branch the mirror pulls that has diverged" +
				" from that repository is replaced by its version, and the replaced commits cannot be recovered through the API.",
		},
		{
			name: "the question for a mirror the call widens, its branches another catalog text with a value of its own",
			msg: New(ConfirmPullMirrorOverwriteWiden,
				S("project", "42"),
				M("branches", New(ConfirmPullMirrorBranchesMatching, S("regex", `"^release/"`))),
				M("source", New(ConfirmPullMirrorConfiguredSource))),
			spanish: "¿Dejar que el mirror de pull del proyecto 42, que sobrescribe las ramas divergentes," +
				` traiga las ramas cuyo nombre coincide con "^release/" desde su origen de mirror configurado?` +
				" Cada rama que trae y que haya divergido de ese repositorio, incluidas las que hasta ahora saltaba," +
				" se sustituye por su versión en la próxima sincronización, y los commits sustituidos no se pueden recuperar mediante la API.",
			english: "Let the pull mirror of project 42, which overwrites diverged branches," +
				` pull the branches whose names match "^release/" from its configured mirror source?` +
				" Every branch it pulls that has diverged from that repository, the ones it skipped until now included," +
				" is replaced by its version on the next sync, and the replaced commits cannot be recovered through the API.",
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
			name: "the push rule question for a project, each change a fragment that continues the sentence",
			msg: New(ConfirmPushRuleRelax,
				M("target", New(ConfirmPushRuleProject, S("id", "group/app"))),
				L("changes",
					New(ConfirmPushRuleSwitchOff, S("setting", "prevent_secrets")),
					New(ConfirmPushRulePatternRemoved, S("setting", "file_name_regex")),
					New(ConfirmPushRulePatternReplaced, S("setting", "branch_name_regex")),
					New(ConfirmPushRuleSizeRaised, N("from", 50), N("to", 100)))),
			spanish: "¿Relajar las push rules del proyecto group/app? Esta llamada desactiva prevent_secrets y quita file_name_regex" +
				" y sustituye branch_name_regex por otro patrón que puede aceptar más y sube max_file_size de 50 MB a 100 MB." +
				" Los push que esas reglas rechazan hoy se aceptan desde entonces.",
			english: "Relax the push rules of project group/app? This call turns off prevent_secrets and removes file_name_regex" +
				" and replaces branch_name_regex with another pattern that may accept more and raises max_file_size from 50 MB to 100 MB." +
				" Pushes those rules reject today are accepted from then on.",
		},
		{
			name: "the push rule question for a group whose rules could not be read",
			msg: New(ConfirmPushRuleRelax,
				M("target", New(ConfirmPushRuleGroup, S("id", "acme"))),
				L("changes",
					New(ConfirmPushRuleUnread, S("setting", "max_file_size")),
					New(ConfirmPushRuleSizeRemoved, N("from", 50)))),
			spanish: "¿Relajar las push rules del grupo acme? Esta llamada puede relajar max_file_size (no se pudieron leer las reglas actuales)" +
				" y quita el límite max_file_size de 50 MB. Los push que esas reglas rechazan hoy se aceptan desde entonces.",
			english: "Relax the push rules of group acme? This call may relax max_file_size (the current rules could not be read)" +
				" and removes the max_file_size limit of 50 MB. Pushes those rules reject today are accepted from then on.",
		},
		{
			name: "the variable question, its target a catalog text with its article",
			msg:  New(ConfirmVariableValue, S("key", "DEPLOY_URL"), M("target", New(ConfirmTargetInstance))),
			spanish: "¿Sustituir el valor de la variable de CI/CD DEPLOY_URL en esta instancia de GitLab?" +
				" El valor que tiene ahora se sobrescribe y ninguna llamada lo restaura.",
			english: "Replace the value of the CI/CD variable DEPLOY_URL in this GitLab instance?" +
				" The value it holds now is overwritten and no call restores it.",
		},
		{
			name: "the user access question, each change a fragment that continues the sentence",
			msg: New(ConfirmUserAccess, S("id", "42"),
				L("changes",
					New(ConfirmUserAccessPassword),
					New(ConfirmUserAccessEmail, S("email", "new@example.com")),
					New(ConfirmUserAccessIdentity))),
			spanish: "¿Cambiar cómo inicia sesión o recupera la cuenta el usuario 42? Esta llamada sustituye la contraseña, que ninguna llamada lee ni restaura" +
				" y pone como dirección de correo new@example.com, cuyo buzón puede entonces restablecer la contraseña" +
				" y sustituye la identidad externa con la que la cuenta inicia sesión.",
			english: "Change how user 42 signs in or recovers the account? This call replaces the password, which no call reads or restores" +
				" and sets the email address to new@example.com, whose mailbox can then reset the password" +
				" and replaces the external identity the account signs in with.",
		},
		{
			name: "the webhook question for a group, a read hook",
			msg: New(ConfirmHookDelivery, S("hook", "7"), M("target", New(ConfirmTargetGroup, S("id", "acme"))),
				L("changes",
					New(ConfirmHookURL, S("url", "https://new.example"), S("current", "https://old.example")),
					New(ConfirmHookToken, S("token", "token")),
					New(ConfirmHookValue, S("field", "custom_headers"), S("key", "Authorization")))),
			spanish: "¿Cambiar adónde entrega el webhook 7 en el grupo acme, o un secreto que envía?" +
				" Esta llamada lo apunta a https://new.example en lugar de https://old.example" +
				" y sustituye su token, que no se puede volver a leer" +
				" y sustituye la entrada Authorization de custom_headers, cuyo valor actual no se puede volver a leer.",
			english: "Change where webhook 7 in group acme delivers, or a secret it sends?" +
				" This call points it at https://new.example instead of https://old.example" +
				" and replaces its token, which cannot be read back" +
				" and replaces the custom_headers entry Authorization, whose current value cannot be read back.",
		},
		{
			name: "the webhook question for an address on the host the hook already uses",
			msg: New(ConfirmHookDelivery, S("hook", "7"), M("target", New(ConfirmTargetInstance)),
				L("changes", New(ConfirmHookURLSameHost, S("url", "https://old.example")))),
			spanish: "¿Cambiar adónde entrega el webhook 7 en esta instancia de GitLab, o un secreto que envía?" +
				" Esta llamada lo mantiene en https://old.example y cambia la ruta o la query a la que entrega ahí.",
			english: "Change where webhook 7 in this GitLab instance delivers, or a secret it sends?" +
				" This call keeps it on https://old.example and changes the path or the query it delivers to there.",
		},
		{
			name: "the webhook question for a project whose hook could not be read",
			msg: New(ConfirmHookDelivery, S("hook", "7"), M("target", New(ConfirmTargetProject, S("id", "group/app"))),
				L("changes",
					New(ConfirmHookURLUnread, S("url", "https://new.example")),
					New(ConfirmHookValueUnread, S("field", "url_variables"), S("key", "tenant")))),
			spanish: "¿Cambiar adónde entrega el webhook 7 en el proyecto group/app, o un secreto que envía?" +
				" Esta llamada puede apuntarlo a otro host, https://new.example (no se pudo leer el webhook)" +
				" y puede sustituir la entrada tenant de url_variables (no se pudo leer el webhook).",
			english: "Change where webhook 7 in project group/app delivers, or a secret it sends?" +
				" This call may point it at another host, https://new.example (the webhook could not be read)" +
				" and may replace the url_variables entry tenant (the webhook could not be read).",
		},
		{
			name: "the status check question",
			msg: New(ConfirmStatusCheck, S("check", "3"), M("target", New(ConfirmTargetProject, S("id", "group/app"))),
				L("changes", New(ConfirmStatusCheckURL, S("url", "https://new.example")), New(ConfirmStatusCheckSecret))),
			spanish: "¿Cambiar adónde envía los merge requests el external status check 3 en el proyecto group/app, o su secreto compartido?" +
				" Esta llamada lo apunta a https://new.example y sustituye su secreto compartido, que no se puede volver a leer.",
			english: "Change where external status check 3 in project group/app sends merge requests, or its shared secret?" +
				" This call points it at https://new.example and replaces its shared secret, which cannot be read back.",
		},
		{
			name: "the quick action question, each command a fragment that quotes its line",
			msg: New(ConfirmQuickActions, L("effects",
				New(ConfirmQuickActionMove, S("line", "`/move group/app`")),
				New(ConfirmQuickActionMerge, S("line", "`/merge`")),
				New(ConfirmQuickActionPromote, S("line", "`/promote`")),
				New(ConfirmQuickActionTicket, S("line", "`/convert_to_ticket a@example.com`")),
				New(ConfirmQuickActionAddEmail, S("line", "`/add_email b@example.com`")),
				New(ConfirmQuickActionPublish, S("line", "`/publish`")),
				New(ConfirmQuickActionUnapprove, S("line", "`/unapprove`")),
				New(ConfirmQuickActionUnlink, S("line", "`/unlink #3`")),
				New(ConfirmQuickActionRemoveParent, S("line", "`/remove_epic`")),
				New(ConfirmQuickActionRemoveChild, S("line", "`/remove_child #4`")),
				New(ConfirmQuickActionMore, N("count", 2)))),
			spanish: "¿Enviar este texto con las quick actions que lleva? GitLab las ejecuta al guardar el texto, así que esta llamada" +
				" mueve el elemento a otro proyecto o grupo (`/move group/app`)" +
				" y fusiona el merge request, de inmediato o cuando pasen sus comprobaciones (`/merge`)" +
				" y cierra el issue y hace de él una épica (`/promote`)" +
				" y convierte el issue en un ticket de Service Desk cuyo autor es una dirección de correo externa (`/convert_to_ticket a@example.com`)" +
				" y añade direcciones de correo externas que reciben los comentarios del issue (`/add_email b@example.com`)" +
				" y publica el issue en la página de estado (`/publish`)" +
				" y retira la aprobación del merge request de quien llama (`/unapprove`)" +
				" y quita el enlace entre este elemento y otro (`/unlink #3`)" +
				" y saca el elemento de su épica o de su padre (`/remove_epic`)" +
				" y saca un hijo del elemento (`/remove_child #4`)" +
				" y ejecuta 2 quick actions más de estos tipos.",
			english: "Send this text with the quick actions it carries? GitLab runs them when it saves the text, so this call" +
				" moves the item to another project or group (`/move group/app`)" +
				" and merges the merge request, at once or when its checks pass (`/merge`)" +
				" and closes the issue and makes an epic of it (`/promote`)" +
				" and converts the issue to a Service Desk ticket whose author is an outside email address (`/convert_to_ticket a@example.com`)" +
				" and adds outside email addresses that receive the comments of the issue (`/add_email b@example.com`)" +
				" and publishes the issue on the status page (`/publish`)" +
				" and withdraws the caller's approval of the merge request (`/unapprove`)" +
				" and removes the link between this item and another (`/unlink #3`)" +
				" and takes the item out of its epic or its parent (`/remove_epic`)" +
				" and takes a child out of the item (`/remove_child #4`)" +
				" and runs 2 more quick actions of these kinds.",
		},
		{
			name: "the three quick actions that empty a list",
			msg: New(ConfirmQuickActions, L("effects",
				New(ConfirmQuickActionUnassign, S("line", "`/unassign`")),
				New(ConfirmQuickActionReassign, S("line", "`/reassign ,`")),
				New(ConfirmQuickActionRemoveContacts, S("line", "`/remove_contacts [contact:a@example.com]`")))),
			spanish: "¿Enviar este texto con las quick actions que lleva? GitLab las ejecuta al guardar el texto, así que esta llamada" +
				" quita asignados del elemento (`/unassign`)" +
				" y quita todos los asignados del elemento (`/reassign ,`)" +
				" y quita contactos CRM del elemento (`/remove_contacts [contact:a@example.com]`).",
			english: "Send this text with the quick actions it carries? GitLab runs them when it saves the text, so this call" +
				" removes assignees from the item (`/unassign`)" +
				" and removes every assignee of the item (`/reassign ,`)" +
				" and removes CRM contacts from the item (`/remove_contacts [contact:a@example.com]`).",
		},
		{
			name:    "the issue epic question",
			msg:     New(ConfirmIssueEpicRemove, N("iid", 7), M("target", New(ConfirmTargetProject, S("id", "`group/app`")))),
			spanish: "¿Sacar de su épica el issue 7 en el proyecto `group/app`? El issue deja de ser hijo de la épica, como cuando se quita de ella.",
			english: "Take issue 7 in project `group/app` out of its epic? The issue is no longer a child of the epic, as when it is removed from the epic.",
		},
		{
			name:    "the work item parent question",
			msg:     New(ConfirmWorkItemParentRemove, S("path", "`group/app`"), N("iid", 7)),
			spanish: "¿Sacar de su padre el work item `group/app`#7? El work item deja de ser hijo de su padre, como cuando se quita de él.",
			english: "Take work item `group/app`#7 out of its parent? The work item is no longer a child of its parent, as when it is removed from the parent.",
		},
		{
			name:    "the epic parent question",
			msg:     New(ConfirmEpicParentRemove, S("path", "`group`"), N("iid", 7)),
			spanish: "¿Sacar de su épica padre la épica `group`&7? La épica deja de ser hija de su padre, como cuando se quita de él.",
			english: "Take epic `group`&7 out of its parent epic? The epic is no longer a child of its parent, as when it is removed from the parent.",
		},
		{
			name: "the issue assignees question",
			msg: New(ConfirmIssueAssigneesClear, L("losses", New(ConfirmWorkItemAssignees, N("count", 2))), N("iid", 7),
				M("target", New(ConfirmTargetProject, S("id", "`group/app`")))),
			spanish: "¿Quitar todos los asignados actuales (2) del issue 7 en el proyecto `group/app`?" +
				" Esto sustituye la lista y no se puede deshacer desde la respuesta.",
			english: "Remove all 2 assignees from issue 7 in project `group/app`?" +
				" This replaces the list and cannot be undone from the response.",
		},
		{
			name: "the merge request assignees question",
			msg: New(ConfirmMergeRequestAssigneesClear, L("losses", New(ConfirmWorkItemAssigneesUnknown)), N("iid", 7),
				M("target", New(ConfirmTargetProject, S("id", "`group/app`")))),
			spanish: "¿Quitar todos los asignados (no se pudo leer la lista actual) del merge request 7 en el proyecto `group/app`?" +
				" Esto sustituye la lista y no se puede deshacer desde la respuesta.",
			english: "Remove every assignee (the current list could not be read) from merge request 7 in project `group/app`?" +
				" This replaces the list and cannot be undone from the response.",
		},
		{
			name:    "the epic assignees question",
			msg:     New(ConfirmEpicAssigneesClear, L("losses", New(ConfirmWorkItemAssignees, N("count", 2))), S("path", "`group`"), N("iid", 7)),
			spanish: "¿Quitar todos los asignados actuales (2) de la épica `group`&7? Esto sustituye la lista y no se puede deshacer desde la respuesta.",
			english: "Remove all 2 assignees from epic `group`&7? This replaces the list and cannot be undone from the response.",
		},
		{
			name: "the import question",
			msg:  New(ConfirmImportOverwrite, S("path", "acme/app")),
			spanish: "¿Importar en acme/app y sobrescribir el proyecto que ya está ahí?" +
				" GitLab sustituye el proyecto existente por el archivo, y ninguna llamada lo devuelve.",
			english: "Import into acme/app and overwrite the project that is already there?" +
				" GitLab replaces the existing project with the archive, and no call brings it back.",
		},
		{
			name: "the forced commit question",
			msg:  New(ConfirmCommitForce, S("branch", "main"), M("target", New(ConfirmTargetProject, S("id", "group/app")))),
			spanish: "¿Crear este commit en la rama main en el proyecto group/app con force?" +
				" La rama se restablece a su punto de partida más el commit nuevo," +
				" y los commits que tiene y el punto de partida no tiene dejan de estar en ella.",
			english: "Create this commit on branch main in project group/app with force?" +
				" The branch is reset to its starting point plus the new commit," +
				" and the commits it holds that the starting point does not are dropped from it.",
		},
		{
			name: "the export question",
			msg: New(ConfirmExportUpload, M("target", New(ConfirmTargetProject, S("id", "group/app"))),
				S("url", "https://files.example/...")),
			spanish: "¿Exportar el proyecto group/app y que GitLab suba el archivo a https://files.example/...?" +
				" Ese host recibe el proyecto entero: su repositorio, issues, merge requests y ajustes.",
			english: "Export project group/app and have GitLab upload the archive to https://files.example/...?" +
				" That host receives the whole project: its repository, issues, merge requests and settings.",
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
