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

	ConfirmDestructiveStandalone:      "¿Confirmas la acción destructiva {tool}?",
	ConfirmDestructiveMeta:            "¿Confirmas {tool}/{action}? Esta acción puede ser irreversible.",
	ConfirmDestructiveIndividual:      "¿Confirmas {tool}? Esta acción puede ser irreversible.",
	ConfirmPullMirrorOverwrite:        "¿Configurar el proyecto {project} para hacer pull desde {source} y sobrescribir las ramas divergentes? Cada rama de este proyecto que haya divergido de ese repositorio se sustituye por su versión en la próxima sincronización, y los commits sustituidos no se pueden recuperar mediante la API.",
	ConfirmPullMirrorConfiguredSource: "su origen de mirror configurado",
	ConfirmWorkItemClear:              "¿Quitar {losses} del work item {path}#{iid}? Esto sustituye la lista y no se puede deshacer desde la respuesta.",
	ConfirmWorkItemAssigneesUnknown:   "todos los asignados (no se pudo leer la lista actual)",
	ConfirmWorkItemAssignees:          "todos los asignados actuales ({count})",
	ConfirmWorkItemCrmContacts:        "todos los contactos CRM",

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
