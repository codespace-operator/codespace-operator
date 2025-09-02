const nonReleasingTypes = /^(docs|chore|build|ci|test|refactor)$/;

// Regex that matches if 'crd' or 'crds' appears anywhere in comma-separated scopes
const operatorScopes = /(?:^|,)\s*(operator|server|ui)\s*(?:,|$)/;

module.exports = {
  branches: ['main'],
  tagFormat: 'operator-${version}',
  plugins: [
    ['@semantic-release/commit-analyzer', {
      preset: 'conventionalcommits',
      parserOpts: { noteKeywords: ['BREAKING CHANGE', 'BREAKING CHANGES', 'BREAKING'] },
      releaseRules: [
        { breaking: true, scope: operatorScopes, release: 'major' },
        { type: 'feat',   scope: operatorScopes, release: 'minor' },
        { type: 'fix',    scope: operatorScopes, release: 'patch' },
        { type: 'perf',   scope: operatorScopes, release: 'patch' },
        { type: 'revert', scope: operatorScopes, release: 'patch' },
        { type: nonReleasingTypes, release: false },
        // Prevent preset defaults from releasing on unrelated scopes
        { type: /.*/, release: false }
      ]
    }],
    '@semantic-release/release-notes-generator',
    ['@semantic-release/changelog', { changelogFile: 'changelogs/CHANGELOG.codespace.md' }],
    [
      '@semantic-release/exec',
      {
        publishCmd: [
          'make docker-buildx IMG=ghcr.io/codespace-operator/codespace-operator:${nextRelease.version} IMG_LATEST=ghcr.io/codespace-operator/codespace-operator:latest',
          'make docker-buildx-server SERVER_IMG=ghcr.io/codespace-operator/codespace-server:${nextRelease.version} SERVER_IMG_LATEST=ghcr.io/codespace-operator/codespace-server:latest',
        ].join(' && '),
      },
    ],
    [
      '@semantic-release/git',
      {
        assets: ['changelogs/CHANGELOG.codespace.md'],
        message: 'chore(release): operator ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}',
      },
    ],
    '@semantic-release/github',
  ],
};
