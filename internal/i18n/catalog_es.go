// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package i18n

// spanish is the first translation: exactly the keys of the English catalog,
// each with the same placeholders, line breaks, bold markers, backticks and
// arrows, which the catalog tests hold every translation to.
//
// The wording follows a glossary taken from the Spanish half of the
// documentation site: what most of its pages write, and what its elicitation
// and progress pages, which a person reads beside these texts, use. The
// person is addressed as tú, the GitLab nouns the site keeps in English stay
// English with the gender it gives them (el issue, el merge request, la
// release, el pipeline, el job, el work item), and the other terms are rama
// origen, rama destino, rama predeterminada, etiquetas, archivo, subida and
// sondeo. An ellipsis is three dots, as in English, and a field title is a
// word rather than the property key English shows: the key itself is never
// translated, since the answer carries it back.
//
// Each template is one string literal, however long, for the reason the
// English catalog gives.
//
//nolint:gosec // G101 fires on the keys named for a password, a token and a secret; these are the questions that name them, and no credential is anywhere near them.
var spanish = map[Key]string{
	ElicitationSchemaConfirmTitle:    "Confirmar",
	ElicitationSchemaSelectionTitle:  "Selección",
	ElicitationSchemaSelectionsTitle: "Selecciones",

	InteractiveFieldTitle:         "Título",
	InteractiveFieldDescription:   "Descripción",
	InteractiveFieldLabels:        "Etiquetas",
	InteractiveFieldSourceBranch:  "Rama origen",
	InteractiveFieldTargetBranch:  "Rama destino",
	InteractiveFieldTagName:       "Tag",
	InteractiveFieldName:          "Nombre",
	InteractiveFieldDefaultBranch: "Rama predeterminada",

	InteractivePromptLabels:           "Escribe las etiquetas separadas por comas (o deja el campo vacío)",
	InteractiveSummaryDescription:     "\n**Descripción**: {description}",
	InteractiveSummaryLabels:          "\n**Etiquetas**: {labels}",
	InteractiveProgressOptionalFields: "Recopilando los campos opcionales...",

	InteractiveIssueCreateProgressDetails:     "Recopilando los datos del issue...",
	InteractiveIssueCreatePromptTitle:         "Escribe el título del issue",
	InteractiveIssueCreatePromptDescription:   "Escribe la descripción del issue (admite Markdown, o deja el campo vacío)",
	InteractiveIssueCreatePromptConfidential:  "¿Debe ser confidencial este issue?",
	InteractiveIssueCreateProgressConfirming:  "Confirmando la creación del issue...",
	InteractiveIssueCreateSummary:             "¿Crear el issue en el proyecto {project}?\n\n**Título**: {title}",
	InteractiveIssueCreateSummaryConfidential: "\n**Confidencial**: Sí",
	InteractiveIssueCreateProgressCreating:    "Creando el issue...",
	InteractiveIssueCreateProgressDone:        "Issue creado",

	InteractiveMrCreateProgressBranches:          "Recopilando la información de las ramas...",
	InteractiveMrCreatePromptSourceBranch:        "Escribe el nombre de la rama origen",
	InteractiveMrCreatePromptTargetBranch:        "Escribe el nombre de la rama destino (por ejemplo, main o develop)",
	InteractiveMrCreateProgressDetails:           "Recopilando los datos del MR...",
	InteractiveMrCreatePromptTitle:               "Escribe el título del merge request",
	InteractiveMrCreatePromptDescription:         "Escribe la descripción del MR (admite Markdown, o deja el campo vacío)",
	InteractiveMrCreatePromptRemoveSourceBranch:  "¿Eliminar la rama origen después del merge?",
	InteractiveMrCreatePromptSquash:              "¿Hacer squash de los commits en el merge?",
	InteractiveMrCreateProgressConfirming:        "Confirmando la creación del MR...",
	InteractiveMrCreateSummary:                   "¿Crear el merge request en el proyecto {project}?\n\n**Título**: {title}\n**Origen**: {source} -> **Destino**: {target}",
	InteractiveMrCreateSummaryRemoveSourceBranch: "\n**Eliminar la rama origen**: Sí",
	InteractiveMrCreateSummarySquash:             "\n**Squash de los commits**: Sí",
	InteractiveMrCreateProgressCreating:          "Creando el merge request...",
	InteractiveMrCreateProgressDone:              "Merge request creado",

	InteractiveReleaseCreateProgressDetails:     "Recopilando los datos de la release...",
	InteractiveReleaseCreatePromptTagName:       "Escribe el nombre del tag de la release (ya debe existir)",
	InteractiveReleaseCreatePromptName:          "Escribe el nombre o título de la release",
	InteractiveReleaseCreateProgressDescription: "Recopilando la descripción de la release...",
	InteractiveReleaseCreatePromptDescription:   "Escribe la descripción o las notas de la release (admite Markdown, o deja el campo vacío)",
	InteractiveReleaseCreateProgressConfirming:  "Confirmando la creación de la release...",
	InteractiveReleaseCreateSummary:             "¿Crear la release en el proyecto {project}?\n\n**Tag**: {tag}\n**Nombre**: {name}",
	InteractiveReleaseCreateProgressCreating:    "Creando la release...",
	InteractiveReleaseCreateProgressDone:        "Release creada",

	InteractiveProjectCreateProgressDetails:            "Recopilando los datos del proyecto...",
	InteractiveProjectCreatePromptName:                 "Escribe el nombre del proyecto",
	InteractiveProjectCreatePromptDescription:          "Escribe la descripción del proyecto (o deja el campo vacío)",
	InteractiveProjectCreateProgressSettings:           "Recopilando la configuración del proyecto...",
	InteractiveProjectCreatePromptVisibility:           "Selecciona la visibilidad del proyecto",
	InteractiveProjectCreatePromptInitializeWithReadme: "¿Inicializar el repositorio con un archivo README?",
	InteractiveProjectCreatePromptDefaultBranch:        "Escribe el nombre de la rama predeterminada (o deja el campo vacío para usar la de la instancia)",
	InteractiveProjectCreateProgressConfirming:         "Confirmando la creación del proyecto...",
	InteractiveProjectCreateSummary:                    "¿Crear un proyecto nuevo de GitLab?\n\n**Nombre**: {name}\n**Visibilidad**: {visibility}",
	InteractiveProjectCreateSummaryReadme:              "\n**README**: Sí",
	InteractiveProjectCreateSummaryDefaultBranch:       "\n**Rama predeterminada**: {branch}",
	InteractiveProjectCreateProgressCreating:           "Creando el proyecto...",
	InteractiveProjectCreateProgressDone:               "Proyecto creado",

	ConfirmDestructiveStandalone:       "¿Confirmas la acción destructiva {tool}?",
	ConfirmDestructiveMeta:             "¿Confirmas {tool}/{action}? Esta acción puede ser irreversible.",
	ConfirmDestructiveIndividual:       "¿Confirmas {tool}? Esta acción puede ser irreversible.",
	ConfirmPullMirrorOverwrite:         "¿Configurar el proyecto {project} para hacer pull desde {source} y sobrescribir las ramas divergentes? Cada rama que trae el mirror y que haya divergido de ese repositorio se sustituye por su versión en la próxima sincronización, y los commits sustituidos no se pueden recuperar mediante la API.",
	ConfirmPullMirrorOverwriteWhileOff: "¿Dejar desactivado el mirror de pull del proyecto {project} y configurarlo para sobrescribir las ramas divergentes desde {source} cuando se active? Desde la primera sincronización posterior, cada rama que trae el mirror y que haya divergido de ese repositorio se sustituye por su versión, y los commits sustituidos no se pueden recuperar mediante la API.",
	ConfirmPullMirrorOverwriteWiden:    "¿Dejar que el mirror de pull del proyecto {project}, que sobrescribe las ramas divergentes, traiga {branches} desde {source}? Cada rama que trae y que haya divergido de ese repositorio, incluidas las que hasta ahora saltaba, se sustituye por su versión en la próxima sincronización, y los commits sustituidos no se pueden recuperar mediante la API.",
	ConfirmPullMirrorBranchesAll:       "todas las ramas",
	ConfirmPullMirrorBranchesProtected: "las ramas protegidas",
	ConfirmPullMirrorBranchesMatching:  "las ramas cuyo nombre coincide con {regex}",
	ConfirmPullMirrorConfiguredSource:  "su origen de mirror configurado",
	ConfirmWorkItemClear:               "¿Quitar {losses} del work item {path}#{iid}? Esto sustituye la lista y no se puede deshacer desde la respuesta.",
	ConfirmWorkItemAssigneesUnknown:    "todos los asignados (no se pudo leer la lista actual)",
	ConfirmWorkItemAssignees:           "todos los asignados actuales ({count})",
	ConfirmWorkItemCrmContacts:         "todos los contactos CRM",
	ConfirmPushRuleRelax:               "¿Relajar las push rules del {target}? Esta llamada {changes}. Los push que esas reglas rechazan hoy se aceptan desde entonces.",
	ConfirmPushRuleProject:             "proyecto {id}",
	ConfirmPushRuleGroup:               "grupo {id}",
	ConfirmPushRuleSwitchOff:           "desactiva {setting}",
	ConfirmPushRulePatternRemoved:      "quita {setting}",
	ConfirmPushRulePatternReplaced:     "sustituye {setting} por otro patrón que puede aceptar más",
	ConfirmPushRuleSizeRaised:          "sube max_file_size de {from} MB a {to} MB",
	ConfirmPushRuleSizeRemoved:         "quita el límite max_file_size de {from} MB",
	ConfirmPushRuleUnread:              "puede relajar {setting} (no se pudieron leer las reglas actuales)",
	ConfirmTargetProject:               "el proyecto {id}",
	ConfirmTargetGroup:                 "el grupo {id}",
	ConfirmTargetInstance:              "esta instancia de GitLab",
	ConfirmVariableValue:               "¿Sustituir el valor de la variable de CI/CD {key} en {target}? El valor que tiene ahora se sobrescribe y ninguna llamada lo restaura.",
	ConfirmUserAccess:                  "¿Cambiar cómo inicia sesión o recupera la cuenta el usuario {id}? Esta llamada {changes}.",
	ConfirmUserAccessPassword:          "sustituye la contraseña, que ninguna llamada lee ni restaura",
	ConfirmUserAccessEmail:             "pone como dirección de correo {email}, cuyo buzón puede entonces restablecer la contraseña",
	ConfirmUserAccessIdentity:          "sustituye la identidad externa con la que la cuenta inicia sesión",
	ConfirmHookDelivery:                "¿Cambiar adónde entrega el webhook {hook} en {target}, o un secreto que envía? Esta llamada {changes}.",
	ConfirmHookURL:                     "lo apunta a {url} en lugar de {current}",
	ConfirmHookURLSameHost:             "lo mantiene en {url} y cambia la ruta o la query a la que entrega ahí",
	ConfirmHookURLUnread:               "puede apuntarlo a otro host, {url} (no se pudo leer el webhook)",
	ConfirmHookToken:                   "sustituye su {token}, que no se puede volver a leer",
	ConfirmHookValue:                   "sustituye la entrada {key} de {field}, cuyo valor actual no se puede volver a leer",
	ConfirmHookValueUnread:             "puede sustituir la entrada {key} de {field} (no se pudo leer el webhook)",
	ConfirmImportOverwrite:             "¿Importar en {path} y sobrescribir el proyecto que ya está ahí? GitLab sustituye el proyecto existente por el archivo, y ninguna llamada lo devuelve.",
	ConfirmCommitForce:                 "¿Crear este commit en la rama {branch} en {target} con force? La rama se restablece a su punto de partida más el commit nuevo, y los commits que tiene y el punto de partida no tiene dejan de estar en ella.",
	ConfirmExportUpload:                "¿Exportar {target} y que GitLab suba el archivo a {url}? Ese host recibe el proyecto entero: su repositorio, issues, merge requests y ajustes.",
	ConfirmStatusCheck:                 "¿Cambiar adónde envía los merge requests el external status check {check} en {target}, o su secreto compartido? Esta llamada {changes}.",
	ConfirmStatusCheckURL:              "lo apunta a {url}",
	ConfirmStatusCheckSecret:           "sustituye su secreto compartido, que no se puede volver a leer",
	ConfirmQuickActions:                "¿Enviar este texto con las quick actions que lleva? GitLab las ejecuta al guardar el texto, así que esta llamada {effects}.",
	ConfirmQuickActionMove:             "mueve el elemento a otro proyecto o grupo ({line})",
	ConfirmQuickActionMerge:            "fusiona el merge request, de inmediato o cuando pasen sus comprobaciones ({line})",
	ConfirmQuickActionPromote:          "cierra el issue y hace de él una épica ({line})",
	ConfirmQuickActionTicket:           "convierte el issue en un ticket de Service Desk cuyo autor es una dirección de correo externa ({line})",
	ConfirmQuickActionAddEmail:         "añade direcciones de correo externas que reciben los comentarios del issue ({line})",
	ConfirmQuickActionPublish:          "publica el issue en la página de estado ({line})",
	ConfirmQuickActionUnapprove:        "retira la aprobación del merge request de quien llama ({line})",
	ConfirmQuickActionUnlink:           "quita el enlace entre este elemento y otro ({line})",
	ConfirmQuickActionRemoveParent:     "saca el elemento de su épica o de su padre ({line})",
	ConfirmQuickActionRemoveChild:      "saca un hijo del elemento ({line})",
	ConfirmQuickActionUnassign:         "quita asignados del elemento ({line})",
	ConfirmQuickActionReassign:         "quita todos los asignados del elemento ({line})",
	ConfirmQuickActionRemoveContacts:   "quita contactos CRM del elemento ({line})",
	ConfirmQuickActionMore:             "ejecuta {count} quick actions más de estos tipos",
	ConfirmIssueEpicRemove:             "¿Sacar de su épica el issue {iid} en {target}? El issue deja de ser hijo de la épica, como cuando se quita de ella.",
	ConfirmWorkItemParentRemove:        "¿Sacar de su padre el work item {path}#{iid}? El work item deja de ser hijo de su padre, como cuando se quita de él.",
	ConfirmEpicParentRemove:            "¿Sacar de su épica padre la épica {path}&{iid}? La épica deja de ser hija de su padre, como cuando se quita de él.",
	ConfirmIssueAssigneesClear:         "¿Quitar {losses} del issue {iid} en {target}? Esto sustituye la lista y no se puede deshacer desde la respuesta.",
	ConfirmMergeRequestAssigneesClear:  "¿Quitar {losses} del merge request {iid} en {target}? Esto sustituye la lista y no se puede deshacer desde la respuesta.",
	ConfirmEpicAssigneesClear:          "¿Quitar {losses} de la épica {path}&{iid}? Esto sustituye la lista y no se puede deshacer desde la respuesta.",

	CommonListAnd: " y ",

	ProgressUploadReading:       "Leídos {read} de {total} bytes, preparando la subida",
	ProgressUploadHandover:      "Leídos {read} bytes, subiendo a GitLab",
	ProgressDownloadBytes:       "Descargados {written} bytes",
	ProgressDownloadOfTotal:     "Descargados {written} de {total} bytes",
	ProgressPipelineWaitPoll:    "Sondeando el pipeline #{pipeline} (intento {attempt}, comprobación de estado)...",
	ProgressJobWaitPoll:         "Sondeando el job #{job} (intento {attempt}, comprobación de estado)...",
	ProgressGroupTransferWait:   "Esperando a que GitLab mueva el grupo",
	ProgressProjectTransferWait: "Esperando a que GitLab mueva el proyecto a {namespace}",
	ProgressPackagePublishFile:  "Publicando el archivo {index} de {count}: {name}",
	ProgressPackagePublishDone:  "Publicados {published} de {count} archivos",
}
