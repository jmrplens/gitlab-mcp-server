# Meta-Tool Evaluation Traces

Each JSON file records the exact task prompt, expected route sequence, assistant tool calls, simulated tool results, validation messages, and final summary for one model-backed evaluation attempt. `traces.jsonl` contains the same records as one JSON object per line for batch analysis.

| Run | Task | Final success | First pass | Trace file |
| ---: | --- | --- | --- | --- |
| 1 | MT-093 | Yes | Yes | [run-001-MT-093.json](run-001-MT-093.json) |
| 1 | MT-099 | Yes | Yes | [run-001-MT-099.json](run-001-MT-099.json) |
| 1 | MT-101 | Yes | Yes | [run-001-MT-101.json](run-001-MT-101.json) |
| 1 | MF-004 | Yes | Yes | [run-001-MF-004.json](run-001-MF-004.json) |
| 1 | MF-005 | Yes | Yes | [run-001-MF-005.json](run-001-MF-005.json) |
