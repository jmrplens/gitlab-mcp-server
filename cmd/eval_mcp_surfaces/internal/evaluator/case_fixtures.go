package evaluator

// BootstrapProjectFixture describes the shared project state reused by typed Docker evaluation cases.
var BootstrapProjectFixture = liveCaseFixture("bootstrap_project", FixtureScopeBootstrap, []string{"project_id", "project_path", "default_branch"}, nil, "project")
