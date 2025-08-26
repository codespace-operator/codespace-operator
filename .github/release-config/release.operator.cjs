module.exports = {
  branches: ['main'],
  tagFormat: 'app-v${version}',
  plugins: [
    ['@semantic-release/commit-analyzer', {
      preset: 'conventionalcommits',
      parserOpts: { noteKeywords: ['BREAKING CHANGE', 'BREAKING CHANGES', 'BREAKING'] },
      releaseRules: [
        { breaking: true, scope: /(^|,|\s)(operator|controller|server|ui|rbac|oidc|ldap|api|ui)(?=,|\s|$)/, release: 'major' },
        { type: 'feat',   scope: /(^|,|\s)(operator|controller|server|ui|rbac|oidc|ldap|api|ui)(?=,|\s|$)/, release: 'minor' },
        { type: 'fix',    scope: /(^|,|\s)(operator|controller|server|ui|rbac|oidc|ldap|api|ui)(?=,|\s|$)/, release: 'patch' },
        { type: 'perf',   scope: /(^|,|\s)(operator|controller|server|ui|rbac|oidc|ldap|api|ui)(?=,|\s|$)/, release: 'patch' },
        { type: 'revert', scope: /(^|,|\s)(operator|controller|server|ui|rbac|oidc|ldap|api|ui)(?=,|\s|$)/, release: 'patch' },
        { scope: /(^|,|\s)(crds|crd|repo|ci)(?=,|\s|$)/, release: false },
        { type: /^(docs|chore|build|ci|test|refactor)$/,     release: false }
      ]
    }],
    '@semantic-release/release-notes-generator',
    ['@semantic-release/changelog', { changelogFile: 'CHANGELOG.app.md' }],
    ['@semantic-release/exec', {
      prepareCmd: [
        'set -e',
        'make build-ui'
      ].join(' && '),
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
      assets: ['CHANGELOG.app.md'],
      message: 'chore(release): app ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}'
    }],
    '@semantic-release/github'
  ]
};
