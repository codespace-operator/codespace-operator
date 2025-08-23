module.exports = {
  branches: ['main'],
  tagFormat: 'crd-v${version}',
  plugins: [
      ['@semantic-release/commit-analyzer', {
      preset: 'conventionalcommits',
      parserOpts: { noteKeywords: ['BREAKING CHANGE', 'BREAKING CHANGES', 'BREAKING'] },
      releaseRules: [
        { breaking: true, scope: '/(^|,|\\s)(crd|api)(?=,|\\s|$)/', release: 'major' },
        { type: 'feat',   scope: '/(^|,|\\s)(crd|api)(?=,|\\s|$)/', release: 'minor' },
        { type: 'fix',    scope: '/(^|,|\\s)(crd|api)(?=,|\\s|$)/', release: 'patch' },
        { type: 'perf',   scope: '/(^|,|\\s)(crd|api)(?=,|\\s|$)/', release: 'patch' },
        { type: 'revert', scope: '/(^|,|\\s)(crd|api)(?=,|\\s|$)/', release: 'patch' },

        { scope: '/(^|,|\\s)(operator|controller|server|ui|chart|helm)(?=,|\\s|$)/', release: false },
        { type: '/^(docs|chore|build|ci|test|refactor)$/',                            release: false }
      ]
    }],
    '@semantic-release/release-notes-generator',
    ['@semantic-release/changelog', { changelogFile: 'CHANGELOG.crd.md' }],
    ['@semantic-release/exec', {
      prepareCmd: [
        'set -e',
        'make manifests',
        'make kustomize',
        'mkdir -p dist',
        './bin/kustomize build config/crd > dist/codespace-operator-crds.yaml',
        'tar -czf dist/codespace-operator-crds-${nextRelease.version}.tgz -C config/crd bases',
        'cd dist && sha256sum codespace-operator-crds.yaml codespace-operator-crds-${nextRelease.version}.tgz > SHA256SUMS.txt'
      ].join(' && ')
    }],
    ['@semantic-release/git', {
      assets: ['CHANGELOG.crd.md'],
      message: 'chore(release): crd ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}'
    }],
    ['@semantic-release/github', {
      assets: [
        { path: 'dist/codespace-operator-crds.yaml', label: 'codespace-operator-crds.yaml' },
        { path: 'dist/codespace-operator-crds-${nextRelease.version}.tgz', label: 'codespace-operator-crds-${nextRelease.version}.tgz' },
        { path: 'dist/SHA256SUMS.txt', label: 'SHA256SUMS.txt' }
      ]
    }]
  ]
};
