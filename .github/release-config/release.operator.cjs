// .github/release-config/release.operator.cjs
const { buildTypeRules, buildBreakingRules } = require('./utils');

const OPERATOR_TOKENS = ['operator', 'controller', 'server', 'ui', 'rbac', 'oidc', 'ldap', 'api'];
const nonReleasingTypes = /^(docs|chore|build|ci|test|refactor)$/;

module.exports = {
  branches: ['main'],
  tagFormat: 'codespace-${version}',
  plugins: [
    [
      '@semantic-release/commit-analyzer',
      {
        preset: 'conventionalcommits',
        parserOpts: { noteKeywords: ['BREAKING CHANGE', 'BREAKING CHANGES', 'BREAKING'] },
        releaseRules: [
          // breaking scopes → major
          ...buildBreakingRules(OPERATOR_TOKENS, 'major'),

          // feat/fix/perf/revert for operator scopes
          ...buildTypeRules(OPERATOR_TOKENS, 'feat',   'minor'),
          ...buildTypeRules(OPERATOR_TOKENS, 'fix',    'patch'),
          ...buildTypeRules(OPERATOR_TOKENS, 'perf',   'patch'),
          ...buildTypeRules(OPERATOR_TOKENS, 'revert', 'patch'),

          // never release on these types
          { type: nonReleasingTypes, release: false },

          // default: do not release if not matched above
          { type: /.*/, release: false },
        ],
      },
    ],
    '@semantic-release/release-notes-generator',
    ['@semantic-release/changelog', { changelogFile: 'changelogs/CHANGELOG.codespace.md' }],
    [
      '@semantic-release/exec',
      {
        publishCmd: [
          'make docker-buildx IMG=ghcr.io/codespace-operator/codespace-operator:${nextRelease.version}',
          'docker tag ghcr.io/codespace-operator/codespace-operator:${nextRelease.version} ghcr.io/codespace-operator/codespace-operator:latest',
          'docker push ghcr.io/codespace-operator/codespace-operator:latest',
          'make docker-build-server SERVER_IMG=ghcr.io/codespace-operator/codespace-server:${nextRelease.version}',
          'docker push ghcr.io/codespace-operator/codespace-server:${nextRelease.version}',
          'docker tag ghcr.io/codespace-operator/codespace-server:${nextRelease.version} ghcr.io/codespace-operator/codespace-server:latest',
          'docker push ghcr.io/codespace-operator/codespace-server:latest',
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
