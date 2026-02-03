# ACCEPTANCE.md

## Manual test

1) Configure root
2) Add AzDO target (org + project)
3) Store PAT in keyring
4) Run sync:
   - clones missing repos into <root>/azure-devops/<org>/<project>/<repo>
   - re-run sync:
     - if clean: fast-forward only
     - if dirty: skip modifying working tree
