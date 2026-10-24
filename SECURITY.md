# Security policy

This project is preparing its initial release. Security fixes target the current
main branch and latest published release; no older-version support is promised.

Report a suspected vulnerability privately through the repository's GitHub
Security tab using **Report a vulnerability**, when private reporting is enabled:
https://github.com/madarlevi/makemkv-tui/security/advisories/new

If that control is unavailable, open a public issue containing only a request for
a private reporting channel, without exploit details, private paths or sample media.
The maintainer must enable private reporting before public release. There is no
promised response time or paid support agreement.

Include the frontend version, OS, filesystem/mount type, sanitized reproduction
steps and expected impact. Preserve affected JSON/staging files locally. Never
post engine keys, personal logs or copyrighted media. Report engine vulnerabilities
to the engine vendor rather than implying the frontend controls engine internals.

Downloaded release checksums detect corruption but are not signatures. Obtain
archives and SHA256SUMS from the intended GitHub release. The frontend executes
external programs on PATH; use trusted engine and optional remux installations.
