// .github/release-config/utils.js
const uniq = (arr) => [...new Set(arr)];

/**
 * Micromatch globs that match a token inside comma-separated scopes.
 * Covers: "token", "token,foo", "foo,token", "foo,token,bar"
 * …and the same with a space after the comma ("foo, token").
 */
function globsForToken(token) {
  return uniq([
    token,
    `${token},*`,
    `*,${token}`,
    `*,${token},*`,
    // space-after-comma variants (some teams allow "scope1, scope2")
    `${token}, *`,
    `*, ${token}`,
    `*, ${token},*`,
  ]);
}

/**
 * Build releaseRules entries for a set of tokens and a semantic-release "type".
 * Example: buildTypeRules(['operator','server'], 'feat', 'minor')
 */
function buildTypeRules(tokens, type, release) {
  return tokens.flatMap((t) =>
    globsForToken(t).map((scope) => ({ type, scope, release }))
  );
}

/**
 * Build "breaking: true" rules for a set of tokens.
 */
function buildBreakingRules(tokens, release = 'major') {
  return tokens.flatMap((t) =>
    globsForToken(t).map((scope) => ({ breaking: true, scope, release }))
  );
}

module.exports = { globsForToken, buildTypeRules, buildBreakingRules };
