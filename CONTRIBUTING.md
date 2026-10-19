# Contributing

Keep credentials and private deployment details out of source, tests, fixtures,
patches, documentation, and assets. Use environment variables for secrets and
reserved example domains or documentation IP addresses in examples. Use explicit
placeholder token values such as `sk-your-api-key-placeholder`.

Run `make sensitive` (or `python3 tools/check_sensitive.py --all`) before submitting
changes. The standard-library-only scanner checks recognizable provider tokens,
JWTs, PEM private keys, and credentials in production URLs; reports include only
file, line, and type. It is a limited guard, so review other sensitive values too.
Previously reviewed synthetic test lines have exact path and content fingerprints;
changing a line requires review again. Never exempt an entire test directory or
add a usable credential to this list.

To check the actual staged content, run
`python3 tools/check_sensitive.py --staged`. Optionally enable the supplied local
hook with `git config --local core.hooksPath .githooks`; this replaces the hook
directory for this checkout, so check any existing hook setup first. On Unix,
ensure `.githooks/pre-commit` is executable before enabling it.

Run the scanner tests with
`python3 -m unittest discover -s tools/tests -p 'test_check_sensitive.py'`.
If a real credential was committed, revoke or rotate it promptly; deleting it
from the current tree does not remove it from Git history.
