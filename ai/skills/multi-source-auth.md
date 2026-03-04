# Skill: Multi-Source Auth

## Goal

Support multiple provider accounts concurrently with secure credential handling.

## Checklist

- Treat each source as a first-class object (`source_id`).
- Support multiple active GitHub and Azure sources at the same time.
- Support personal and org/team contexts in source metadata.
- Store PAT values only in OS keyring/credential manager.
- Persist only non-sensitive source metadata in config/local DB.
- Validate PAT at login and return actionable errors.
- Never leak PAT values in logs, events, or CLI output.

## Must-Have Tests

- Two+ GitHub + two+ Azure sources can coexist.
- Repo sync uses the correct source credentials.
- Invalid PAT errors are clear and secret-safe.

## Commit Pattern

1. Source registry model.
2. Credential storage and provider validation.
3. Multi-source routing tests.
