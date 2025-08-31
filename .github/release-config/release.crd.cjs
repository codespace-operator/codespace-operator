
const releaseScope = /(^|,|\s)(crds|crd)(?=,|\s|$)/
const releaseType = /^(docs|chore|build|ci|test|refactor)$/
const notReleaseScope = /(^|,|\s)(repo|ci)(?=,|\s|$)/

module.exports = {
  branches: ['main'],
  tagFormat: 'crd-${version}',
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
        { type: releaseType, release: false }
      ]
    }],
    '@semantic-release/release-notes-generator',
    ['@semantic-release/changelog', { changelogFile: 'changelogs/CHANGELOG.crd.md' }],
    ['@semantic-release/exec', {
      prepareCmd: [
        'set -e',
        'make manifests',
        'make kustomize',
        'mkdir -p dist',
        './bin/kustomize build config/crd > dist/codespace-operator-${nextRelease.version}-crds.yaml',
        'tar -czf dist/codespace-operator-crds-${nextRelease.version}.tgz -C config/crd bases',
        'cd dist && sha256sum codespace-operator-${nextRelease.version}-crds.yaml codespace-operator-crds-${nextRelease.version}.tgz > SHA256SUMS.txt'
      ].join(' && ')
    }],
    ['@semantic-release/git', {
      assets: ['changelogs/CHANGELOG.crd.md'],
      message: 'chore(release): crd ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}'
    }],
    ['@semantic-release/github', {
  assets: [
        'dist/codespace-operator-*-crds.yaml',
        'dist/codespace-operator-crds-*.tgz',
        'dist/SHA256SUMS.txt'
      ]
    }]
  ]
};
