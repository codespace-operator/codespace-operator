// Operator release configuration
// Example commits:
//   feat(operator): add new reconciliation loop
//   fix(ui): button not rendering
//   perf(server): improve query caching
//   chore(operator): update dependencies   <-- no release

const operatorScopes = /^(operator|controller|server|ui|rbac|oidc|ldap|api)$/;
const nonReleasingTypes = /^(docs|chore|build|ci|test|refactor)$/;

module.exports = {
  branches: ['main'],
  tagFormat: 'codespace-${version}',
  plugins: [
    ['@semantic-release/commit-analyzer', {
      preset: 'conventionalcommits',
      parserOpts: { noteKeywords: ['BREAKING CHANGE', 'BREAKING CHANGES', 'BREAKING'] },
      releaseRules: [
        { breaking: true, scope: operatorScopes, release: 'major' }, // BREAKING in operator scope → major
        { type: 'feat',   scope: operatorScopes, release: 'minor' }, // feat(operator) → minor
        { type: 'fix',    scope: operatorScopes, release: 'patch' }, // fix(server) → patch
        { type: 'perf',   scope: operatorScopes, release: 'patch' }, // perf(ui) → patch
        { type: 'revert', scope: operatorScopes, release: 'patch' }, // revert(operator) → patch
        { type: nonReleasingTypes, release: false }                  // docs/chore/ci/etc. → no release
      ]
    }],
    '@semantic-release/release-notes-generator',
    ['@semantic-release/changelog', { changelogFile: 'changelogs/CHANGELOG.codespace.md' }],
    ['@semantic-release/exec', {
      publishCmd: [
        'make docker-buildx IMG=ghcr.io/codespace-operator/codespace-operator:${nextRelease.version}',
        'docker tag ghcr.io/codespace-operator/codespace-operator:${nextRelease.version} ghcr.io/codespace-operator/codespace-operator:latest',
        'docker push ghcr.io/codespace-operator/codespace-operator:latest',
        'make docker-build-server SERVER_IMG=ghcr.io/codespace-operator/codespace-server:${nextRelease.version}',
        'docker push ghcr.io/codespace-operator/codespace-server:${nextRelease.version}',
        'docker tag ghcr.io/codespace-operator/codespace-server:${nextRelease.version} ghcr.io/codespace-operator/codespace-server:latest',
        'docker push ghcr.io/codespace-operator/codespace-server:latest'
      ].join(' && ')
    }],
    ['@semantic-release/git', {
      assets: ['changelogs/CHANGELOG.codespace.md'],
      message: 'chore(release): app ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}'
    }],
    '@semantic-release/github'
  ]
};
