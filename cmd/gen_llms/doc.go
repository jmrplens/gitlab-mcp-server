// Command gen_llms generates llms.txt and llms-full.txt files. It creates an
// in-memory MCP server with all tools, resources, and prompts registered,
// introspects them via the SDK, and writes two files to the project root:
//
//   - llms.txt: concise llmstxt.org index for LLM discovery
//   - llms-full.txt: detailed companion reference with tool schemas
//
// Usage:
//
//	go run ./cmd/gen_llms/
//	go run ./cmd/gen_llms/ --check
package main
