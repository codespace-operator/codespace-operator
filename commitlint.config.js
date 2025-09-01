// commitlint.config.js
// Enforces Conventional Commit rules for PR titles and commit messages
// Examples:
//   feat(operator): add new reconciliation loop
//   fix(crd): correct schema for Foo
//   chore(ci): bump actions/checkout version

module.exports = {
  extends: ['@commitlint/config-conventional'],
  rules: {
    // ✅ Restrict commit scopes to meaningful areas in the repo
    'scope-enum': [2, 'always', [
      'operator',   // Operator core (controllers, binaries, Dockerfile)
      'controller', // Specific controllers inside the operator
      'server',     // Server component / backend service
      'ui',         // Frontend / UI code
      'crd',        // CustomResourceDefinition changes (singular form)
      'crds',       // CustomResourceDefinition changes (plural form)
      'oidc',       // OIDC integration / auth features
      'ldap',       // LDAP integration / auth features
      'rbac',       // RBAC manifests or permission logic
      'api',        // Go API types and client interfaces
      'repo',       // Repo-level housekeeping (release rules, GH workflows)
      'ci',         // CI workflows or pipeline changes
      'test'        // Testing framework or test cases
    ]],

    // ✅ Restrict commit types to Conventional Commit "verbs"
    'type-enum': [2, 'always', [
      'feat',      // New feature
      'fix',       // Bug fix
      'perf',      // Performance improvement
      'refactor',  // Code restructure without changing behavior
      'docs',      // Documentation changes only
      'chore',     // Misc maintenance (deps, cleanup, tooling)
      'ci',        // CI/CD config or script changes
      'build',     // Build system or dependency changes
      'test'       // Adding or adjusting tests
    ]]
  }
};
