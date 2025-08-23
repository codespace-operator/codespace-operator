module.exports = {
  branches: ['main'],
  tagFormat: 'chart-v${version}',
  plugins: [
    ['@semantic-release/commit-analyzer', {
      preset: 'conventionalcommits',
      parserOpts: { noteKeywords: ['BREAKING CHANGE', 'BREAKING CHANGES', 'BREAKING'] },
      releaseRules: [
        { breaking: true, scope: '/(^|,|\\s)(chart|helm)(?=,|\\s|$)/', release: 'major' },
        { type: 'feat',   scope: '/(^|,|\\s)(chart|helm)(?=,|\\s|$)/', release: 'minor' },
        { type: 'fix',    scope: '/(^|,|\\s)(chart|helm)(?=,|\\s|$)/', release: 'patch' },
        { type: 'perf',   scope: '/(^|,|\\s)(chart|helm)(?=,|\\s|$)/', release: 'patch' },
        { type: 'revert', scope: '/(^|,|\\s)(chart|helm)(?=,|\\s|$)/', release: 'patch' },

        { scope: '/(^|,|\\s)(operator|controller|server|ui|crd|api)(?=,|\\s|$)/', release: false },
        { type: '/^(docs|chore|build|ci|test|refactor)$/', release: false }
      ]
    }],
    '@semantic-release/release-notes-generator',
    ['@semantic-release/changelog', { changelogFile: 'CHANGELOG.chart.md' }],
    // publish chart via semantic-release-helm3 (as before)
    ['semantic-release-helm3', {
      chartPath: './helm',
      registry: 'oci://ghcr.io/codespace-operator/charts',
      onlyUpdateVersion: true     // appVersion is driven by app lane
    }],
    ['@semantic-release/git', {
      assets: ['CHANGELOG.chart.md', 'helm/Chart.yaml'],
      message: 'chore(release): chart ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}'
    }],
    '@semantic-release/github'
  ]
};
