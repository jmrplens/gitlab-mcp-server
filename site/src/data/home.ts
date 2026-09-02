/**
 * The landing page's copy, one object per locale.
 *
 * Home.astro renders the structure; this file supplies the words. That split
 * is the whole point: with the two locales as two objects of one type, the
 * English and the Spanish landing cannot drift apart structurally — a block
 * added to one is a type error until it is added to the other.
 *
 * Numbers are not written here. They come from src/data/stats.json — the
 * file `make gen-site-stats` derives from the live catalog — and from
 * src/data/token-footprint.json, which `make gen-footprint` measures with the
 * tokenizer; both are read in Home.astro, so a catalog change updates the
 * landing by itself. The one sentence here that carries figures is a template
 * whose placeholders Home.astro fills from that measurement.
 */

export interface Step {
	title: string;
	body: string;
	code?: string;
	href: string;
	linkText: string;
}

export interface SurfaceRow {
	/** Keys the count lookup in Home.astro; the numbers live in stats.json. */
	id: "dynamic" | "meta" | "individual";
	name: string;
	summary: string;
	when: string;
}

export interface HomeContent {
	/**
	 * BCP 47 tag the counted values are formatted with, so a thousands
	 * separator follows the locale's convention ("10,333" against "10.333").
	 */
	numberLocale: "en-US" | "es-ES";
	statsLabel: string;
	/** The five labels the readout pairs with counted values. */
	statLabels: {
		dynamic: string;
		/** The measured startup context of the default configuration. */
		context: string;
		tools: string;
		prompts: string;
		resources: string;
	};
	statHrefs: {
		dynamic: string;
		context: string;
		tools: string;
		prompts: string;
		resources: string;
	};
	/**
	 * The one-sentence claim under the readout. `{default}` and `{minimal}`
	 * are replaced in Home.astro with the measured startup totals of the
	 * default configuration and of `GITLAB_MCP_CAPABILITY_SURFACE=minimal`, taken from
	 * src/data/token-footprint.json. Rendered as HTML so the link to the
	 * methodology survives.
	 */
	contextClaim: string;
	what: { title: string; body: string[] };
	who: { title: string; items: string[] };
	honest: { title: string; items: string[] };
	proof: {
		title: string;
		lead: string;
		installTitle: string;
		installNote: string;
		install: string;
		promptsTitle: string;
		promptsNote: string;
		promptHeaders: [string, string];
		prompts: [string, string][];
	};
	surfaces: {
		title: string;
		lead: string;
		headers: [string, string, string];
		rows: SurfaceRow[];
		/** Names for the three edition chips the legend renders. */
		tierNames: [string, string, string];
		/** Explains that every count depends on the instance's licence tier. */
		legend: string;
	};
	start: { title: string; lead: string; steps: Step[] };
}

const INSTALL = `claude mcp add gitlab --env GITLAB_URL=https://gitlab.example.com \\
  --env GITLAB_TOKEN=glpat-xxxx --transport stdio \\
  -- docker run -i --rm -e GITLAB_URL -e GITLAB_TOKEN \\
  ghcr.io/jmrplens/gitlab-mcp-server:latest`;

export const en: HomeContent = {
	numberLocale: "en-US",
	statsLabel: "GitLab MCP Server at a glance",
	statLabels: {
		dynamic: "tools in the default dynamic surface",
		context: "tokens of startup context by default",
		tools: "individual tools at the widest tier",
		prompts: "guided prompts",
		resources: "MCP resources",
	},
	statHrefs: {
		dynamic: "/gitlab-mcp-server/tools/dynamic-tools/",
		context:
			"/gitlab-mcp-server/tools/dynamic-tools/#how-much-startup-context-does-dynamic-mode-save",
		tools: "/gitlab-mcp-server/tools/overview/",
		prompts: "/gitlab-mcp-server/tools/resources-prompts/",
		resources: "/gitlab-mcp-server/tools/resources-prompts/",
	},
	contextClaim:
		'The default configuration costs {default} tokens of context on every GitLab tier; {minimal} with <code>GITLAB_MCP_CAPABILITY_SURFACE=minimal</code>. <a href="/gitlab-mcp-server/tools/dynamic-tools/#how-much-startup-context-does-dynamic-mode-save">Measured</a>, not estimated.',
	what: {
		title: "What it is",
		body: [
			'<strong>GitLab MCP Server</strong> is a <a href="https://modelcontextprotocol.io">Model Context Protocol</a> server, written in Go, that lets an AI assistant work your GitLab for you: review merge requests, triage pipelines, manage issues, draft releases. It covers the REST v4 and GraphQL APIs and runs against GitLab.com or any self-hosted instance, Community or Enterprise Edition.',
			"You talk to your assistant, not to the tools. One canonical action catalog is projected into three tool surfaces, so the same operations fit a low-token client, a domain-tool client, or one tool per operation — and the assistant picks arguments from typed schemas, not from prose.",
			"It ships as one static binary (or a container) for Linux, macOS and Windows, speaking stdio for a local client or HTTP for a shared, multi-user deployment.",
		],
	},
	who: {
		title: "Who it is for",
		items: [
			"Anyone who reviews merge requests and would rather ask for the risky diff in a sentence than open six tabs.",
			"People running an MCP client already — Claude Code, Claude Desktop, Cursor, VS Code, or their own agent.",
			"Teams on self-hosted GitLab, Community Edition included: tier-gated tools register only when the instance licence carries them.",
			"Operators who want one shared HTTP endpoint with per-token isolation instead of a binary per laptop.",
		],
	},
	honest: {
		title: "What it is not",
		items: [
			"Not a way around your permissions. Every call is made with your token, and what its scopes and your instance licence forbid stays forbidden.",
			"Not an official GitLab product. It talks to GitLab's documented APIs and says so; GitLab is a trademark of GitLab Inc.",
			"Not obliged to write. Read-only mode removes every mutating action, and safe mode turns each one into a preview instead of an execution.",
			"Not a black box. Every figure on this page is generated from the catalog, and the model-facing behaviour is measured by a live evaluation suite.",
		],
	},
	proof: {
		title: "What it looks like in use",
		lead: "One command to connect it, plain sentences after that.",
		installTitle: "Connect it to Claude Code",
		installNote:
			"Docker keeps it zero-install; a native binary works the same way. Other clients take the same server over stdio or HTTP.",
		install: INSTALL,
		promptsTitle: "Then just ask",
		promptsNote:
			"The assistant chooses the tool and the arguments; the server returns structured JSON for the model and formatted Markdown for you.",
		promptHeaders: ["You say", "The server does"],
		prompts: [
			[
				"“Show open merge requests in <code>my-org/backend</code> that need review”",
				"Lists MRs with authors, assignees and pipeline state, as a table with links.",
			],
			[
				"“Why did the pipeline fail on <code>feature/auth</code>?”",
				"Finds the failing job, reads its log, and summarises the error with a suggested fix.",
			],
			[
				"“Create a P1 bug for the auth regression and assign it to @alice”",
				"Creates the issue with labels and assignee — or, in safe mode, shows the exact call it would make.",
			],
			[
				"“Draft release notes for v2.1.0 against v2.0.0”",
				"Compares the tags, groups the commits and merge requests, and drafts the changelog.",
			],
		],
	},
	surfaces: {
		title: "One catalog, three surfaces",
		lead: "Every operation lives once, in a canonical action catalog, and is projected into the surface that fits your client's context budget. The counts are generated from the catalog, never typed.",
		headers: ["Surface", "Tools", "When to use it"],
		rows: [
			{
				id: "dynamic",
				name: "Dynamic",
				summary: "find_action + execute_action — the default",
				when: "Smallest context footprint: the assistant searches the catalog, reads the exact schema, then executes the canonical action.",
			},
			{
				id: "meta",
				name: "Meta-tools",
				summary: "one dispatcher per domain",
				when: "A visible tool per domain (projects, issues, MRs, pipelines…) with routed actions inside — the balanced middle.",
			},
			{
				id: "individual",
				name: "Individual",
				summary: "one tool per operation",
				when: "Everything visible at once, for clients that prefer a flat list and have the context to hold it.",
			},
		],
		tierNames: ["Free/CE", "Premium", "Ultimate"],
		legend:
			"Ranges span the instance tier: the lower bound is Free/CE, the upper is GitLab.com Ultimate with Orbit. Premium and Ultimate tools register only when the connected instance licence carries them.",
	},
	start: {
		title: "Start here",
		lead: "Three steps from nothing to an assistant with hands on GitLab.",
		steps: [
			{
				title: "Install",
				body: "Docker, Homebrew, winget, a release binary, or the Claude Desktop extension — pick the one your machine likes.",
				href: "/gitlab-mcp-server/getting-started/",
				linkText: "Getting started",
			},
			{
				title: "Point it at GitLab",
				body: "A personal access token and your instance URL are the whole configuration. Both go in your MCP client's own JSON.",
				code: "GITLAB_URL=https://gitlab.example.com\nGITLAB_TOKEN=glpat-xxxx",
				href: "/gitlab-mcp-server/configuration/",
				linkText: "Client configuration",
			},
			{
				title: "Pick a surface",
				body: "The dynamic default fits any client. Prefer domain tools or one tool per operation? One environment variable switches the surface.",
				code: "GITLAB_MCP_TOOL_SURFACE=dynamic|meta|individual",
				href: "/gitlab-mcp-server/tools/overview/",
				linkText: "Tool surfaces",
			},
		],
	},
};

export const es: HomeContent = {
	numberLocale: "es-ES",
	statsLabel: "GitLab MCP Server de un vistazo",
	statLabels: {
		dynamic: "tools en la superficie dinámica por defecto",
		context: "tokens de contexto inicial por defecto",
		tools: "tools individuales en el tier más amplio",
		prompts: "prompts guiados",
		resources: "recursos MCP",
	},
	statHrefs: {
		dynamic: "/gitlab-mcp-server/es/tools/dynamic-tools/",
		context:
			"/gitlab-mcp-server/es/tools/dynamic-tools/#cuánto-contexto-de-arranque-ahorra-el-modo-dinámico",
		tools: "/gitlab-mcp-server/es/tools/overview/",
		prompts: "/gitlab-mcp-server/es/tools/resources-prompts/",
		resources: "/gitlab-mcp-server/es/tools/resources-prompts/",
	},
	contextClaim:
		'La configuración por defecto cuesta {default} tokens de contexto en todos los tiers de GitLab; {minimal} con <code>GITLAB_MCP_CAPABILITY_SURFACE=minimal</code>. <a href="/gitlab-mcp-server/es/tools/dynamic-tools/#cuánto-contexto-de-arranque-ahorra-el-modo-dinámico">Medido</a>, no estimado.',
	what: {
		title: "Qué es",
		body: [
			'<strong>GitLab MCP Server</strong> es un servidor <a href="https://modelcontextprotocol.io">Model Context Protocol</a>, escrito en Go, que permite a un asistente de IA trabajar tu GitLab por ti: revisar merge requests, triar pipelines, gestionar issues, redactar releases. Cubre las APIs REST v4 y GraphQL y funciona contra GitLab.com o cualquier instancia autoalojada, Community o Enterprise Edition.',
			"Tú hablas con tu asistente, no con las herramientas. Un único catálogo canónico de acciones se proyecta en tres superficies de tools, de modo que las mismas operaciones sirven a un cliente de contexto reducido, a uno de tools por dominio o a uno con una tool por operación — y el asistente toma los argumentos de esquemas tipados, no de prosa.",
			"Se distribuye como un binario estático (o un contenedor) para Linux, macOS y Windows, hablando stdio para un cliente local o HTTP para un despliegue compartido multiusuario.",
		],
	},
	who: {
		title: "Para quién es",
		items: [
			"Quien revisa merge requests y prefiere pedir el diff arriesgado en una frase antes que abrir seis pestañas.",
			"Quien ya usa un cliente MCP — Claude Code, Claude Desktop, Cursor, VS Code o su propio agente.",
			"Equipos con GitLab autoalojado, Community Edition incluida: las tools con tier se registran solo cuando la licencia de la instancia las cubre.",
			"Operadores que quieren un único endpoint HTTP compartido con aislamiento por token en lugar de un binario por portátil.",
		],
	},
	honest: {
		title: "Qué no es",
		items: [
			"No es una forma de saltarse tus permisos. Cada llamada se hace con tu token, y lo que sus scopes y la licencia de tu instancia prohíben sigue prohibido.",
			"No es un producto oficial de GitLab. Habla con las APIs documentadas de GitLab y lo dice; GitLab es una marca de GitLab Inc.",
			"No está obligado a escribir. El modo de solo lectura elimina toda acción mutadora, y el modo seguro convierte cada una en una vista previa en lugar de una ejecución.",
			"No es una caja negra. Cada cifra de esta página se genera desde el catálogo, y el comportamiento de cara al modelo se mide con una suite de evaluación real.",
		],
	},
	proof: {
		title: "Cómo se ve en uso",
		lead: "Un comando para conectarlo; frases normales después.",
		installTitle: "Conéctalo a Claude Code",
		installNote:
			"Docker lo deja en cero instalación; un binario nativo funciona igual. Otros clientes usan el mismo servidor por stdio o HTTP.",
		install: INSTALL,
		promptsTitle: "Y después, solo pide",
		promptsNote:
			"El asistente elige la tool y los argumentos; el servidor devuelve JSON estructurado para el modelo y Markdown formateado para ti.",
		promptHeaders: ["Tú dices", "El servidor hace"],
		prompts: [
			[
				"«Muéstrame los merge requests abiertos de <code>my-org/backend</code> pendientes de revisión»",
				"Lista los MRs con autores, asignados y estado del pipeline, en una tabla con enlaces.",
			],
			[
				"«¿Por qué falló el pipeline de <code>feature/auth</code>?»",
				"Encuentra el job fallido, lee su log y resume el error con una propuesta de arreglo.",
			],
			[
				"«Crea un bug P1 para la regresión de auth y asígnaselo a @alice»",
				"Crea la issue con etiquetas y asignación — o, en modo seguro, muestra la llamada exacta que haría.",
			],
			[
				"«Redacta las notas de la release v2.1.0 contra v2.0.0»",
				"Compara los tags, agrupa commits y merge requests, y redacta el changelog.",
			],
		],
	},
	surfaces: {
		title: "Un catálogo, tres superficies",
		lead: "Cada operación vive una sola vez, en un catálogo canónico de acciones, y se proyecta en la superficie que encaja con el presupuesto de contexto de tu cliente. Las cifras se generan desde el catálogo, nunca se escriben a mano.",
		headers: ["Superficie", "Tools", "Cuándo usarla"],
		rows: [
			{
				id: "dynamic",
				name: "Dinámica",
				summary: "find_action + execute_action — la opción por defecto",
				when: "La huella de contexto más pequeña: el asistente busca en el catálogo, lee el esquema exacto y ejecuta la acción canónica.",
			},
			{
				id: "meta",
				name: "Meta-tools",
				summary: "un despachador por dominio",
				when: "Una tool visible por dominio (proyectos, issues, MRs, pipelines…) con acciones enrutadas dentro — el punto medio.",
			},
			{
				id: "individual",
				name: "Individual",
				summary: "una tool por operación",
				when: "Todo visible a la vez, para clientes que prefieren una lista plana y tienen contexto para sostenerla.",
			},
		],
		tierNames: ["Free/CE", "Premium", "Ultimate"],
		legend:
			"Los rangos recorren el tier de la instancia: el límite inferior es Free/CE y el superior, GitLab.com Ultimate con Orbit. Las tools Premium y Ultimate se registran solo cuando la licencia de la instancia conectada las cubre.",
	},
	start: {
		title: "Empieza aquí",
		lead: "Tres pasos entre la nada y un asistente con las manos en GitLab.",
		steps: [
			{
				title: "Instala",
				body: "Docker, Homebrew, winget, un binario de release o la extensión de Claude Desktop — elige lo que le guste a tu máquina.",
				href: "/gitlab-mcp-server/es/getting-started/",
				linkText: "Primeros pasos",
			},
			{
				title: "Apúntalo a GitLab",
				body: "Un token de acceso personal y la URL de tu instancia son toda la configuración. Ambos van en el propio JSON de tu cliente MCP.",
				code: "GITLAB_URL=https://gitlab.example.com\nGITLAB_TOKEN=glpat-xxxx",
				href: "/gitlab-mcp-server/es/configuration/",
				linkText: "Configuración del cliente",
			},
			{
				title: "Elige superficie",
				body: "La dinámica por defecto encaja en cualquier cliente. ¿Prefieres tools por dominio o una por operación? Una variable de entorno cambia la superficie.",
				code: "GITLAB_MCP_TOOL_SURFACE=dynamic|meta|individual",
				href: "/gitlab-mcp-server/es/tools/overview/",
				linkText: "Superficies de tools",
			},
		],
	},
};
