
const releaseScope = /(^|,|\s)(operator|controller|server|ui|rbac|oidc|ldap|api|ui)(?=,|\s|$)/
const releaseType = /^(docs|chore|build|ci|test|refactor)$/
const notReleaseScope = /(^|,|\s)(crds|crd|repo|ci)(?=,|\s|$)/


module.exports = {
  branches: ['main'],
  tagFormat: 'app-v${version}',
  plugins: [
    ['@semantic-release/commit-analyzer', {
      preset: 'conventionalcommits',
      parserOpts: { noteKeywords: ['BREAKING CHANGE', 'BREAKING CHANGES', 'BREAKING'] },
      releaseRules: [
        { breaking: true, scope: releaseScope, release: 'major' },
        { type: 'feat',   scope: releaseScope, release: 'minor' },
        { type: 'fix',    scope: releaseScope, release: 'patch' },
        { type: 'perf',   scope: releaseScope, release: 'patch' },
        { type: 'revert', scope: releaseScope, release: 'patch' },
        { scope: notReleaseScope, release: false },
        { type: releaseType,     release: false }
      ]
    }],
    '@semantic-release/release-notes-generator',
    ['@semantic-release/changelog', { changelogFile: 'CHANGELOG.app.md' }],
    ['@semantic-release/exec', {
      prepareCmd: [
        'set -e',
        'make build-server'
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
