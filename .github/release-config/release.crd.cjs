// CRD release configuration
// Example commits:
//   feat(crd): add new field to Spec
//   fix(crds): correct validation schema
//   perf(crd): optimize generated CRD
//   chore(crd): update kustomize version  <-- no release

const crdScopes = /^(crd|crds)$/;
const nonReleasingTypes = /^(docs|chore|build|ci|test|refactor)$/;

module.exports = {
  branches: ['main'],
  tagFormat: 'crd-${version}',
  plugins: [
    ['@semantic-release/commit-analyzer', {
      preset: 'conventionalcommits',
      parserOpts: { noteKeywords: ['BREAKING CHANGE', 'BREAKING CHANGES', 'BREAKING'] },
      releaseRules: [
        { breaking: true, scope: crdScopes, release: 'major' }, // BREAKING in crd scope → major
        { type: 'feat',   scope: crdScopes, release: 'minor' }, // feat(crd) → minor
        { type: 'fix',    scope: crdScopes, release: 'patch' }, // fix(crds) → patch
        { type: 'perf',   scope: crdScopes, release: 'patch' }, // perf(crd) → patch
        { type: 'revert', scope: crdScopes, release: 'patch' }, // revert(crd) → patch
        { type: nonReleasingTypes, release: false }             // docs/chore/ci/etc. → no release
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
