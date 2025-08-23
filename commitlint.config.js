module.exports = {
  extends: ['@commitlint/config-conventional'],
  rules: {
    'scope-empty': [2, 'never'],
    'scope-enum': [
      2,
      'always',
      [
        // operator lane
        'operator', 'controller', 'server', 'ui',
        // helm lane
        'chart', 'helm',
        // crd lane
        'crd', 'api',
        // misc (no release)
        'repo', 'readme', 'docs', 'build', 'ci', 'deps', 'release', 'test'
      ]
    ]
  }
};
