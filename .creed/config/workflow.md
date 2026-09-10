# Workflow

1. Read the relevant port, provider, use case, and existing tests before changing behavior.
2. Keep domain/use-case logic independent of external providers; add or adjust ports before wiring providers in `internal/bootstrap/`.
3. Keep configuration compatible with the documented precedence order and redact secrets from diagnostics.
4. Add focused tests for behavior changes. Preserve the race and coverage expectations in the CI contract.
5. Run the applicable Make targets, then `make ci` for a complete change.
6. Before committing, inspect `git diff --check`. Lefthook runs staged gitleaks protection; never bypass it.

GitHub Actions is the source of truth for the full OS build matrix. Local Linux desktop builds can require GTK/WebKit dependencies that are absent on a development machine.
