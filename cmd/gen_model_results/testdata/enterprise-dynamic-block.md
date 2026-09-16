Compared as one table because these rows agree on surface `dynamic`, mode `default`, tier `ultimate`, corpus `corpus-1`, contract `contract-1`, tool schemas `tools-1`, repeat 1.

| Model                  | Reached | Accepted first time | Argument fidelity | Confirmation | Unaided | Completion |                             Overhead |
| ---------------------- | ------: | ------------------: | ----------------: | -----------: | ------: | ---------: | -----------------------------------: |
| `anthropic:test-model` | 18 / 20 |             15 / 18 |           22 / 24 |        3 / 4 |  7 / 10 |     8 / 10 |   12 / 18 (9 find, 3 invalid_params) |
| `openai:other-model`   | 14 / 20 |              9 / 14 |           17 / 24 |            - |  3 / 10 |     5 / 10 | 25 / 14 (14 find, 11 invalid_params) |

What the columns leave out: an attempt the instance could not offer, one the server's span never described, one the provider would not answer, one this side broke, and one GitLab refused after a correct dispatch. None of the five is the model's, so none is in any denominator.

| Model                  | Attempts | Turns | Skipped | Unobserved | Provider errors | Harness errors | GitLab refused |
| ---------------------- | -------: | ----: | ------: | ---------: | --------------: | -------------: | -------------: |
| `anthropic:test-model` |       10 |    31 |       2 |          1 |               1 |              0 |              1 |
| `openai:other-model`   |       10 |    40 |       0 |          0 |               0 |              0 |              0 |

Tokens, never folded into one figure: a cache read is not an input token, and a table that added them together is how sixty thousand tokens came to be published against five million.

| Model                  |  Input | Output | Cache created | Cache read |
| ---------------------- | -----: | -----: | ------------: | ---------: |
| `anthropic:test-model` | 120000 |   4200 |         30000 |      88000 |
| `openai:other-model`   | 180000 |   9100 |             0 |          0 |

| Model                  | Provider    | Commit      | Date       | GitLab            | Tools served | Token scopes | Request options                              |
| ---------------------- | ----------- | ----------- | ---------- | ----------------- | -----------: | ------------ | -------------------------------------------- |
| `anthropic:test-model` | `anthropic` | `111111111` | 2026-09-16 | 19.3.0 enterprise |            2 | api          | `max_tokens=4096`, `temperature=0`           |
| `openai:other-model`   | `openai`    | `111111111` | 2026-09-16 | 19.3.0 enterprise |            2 | api          | `max_tokens=4096`, `reasoning_effort=medium` |
