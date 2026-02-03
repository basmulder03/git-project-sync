# ACCEPTANCE.md

## Manual test

1) Configure root (`config init --root <path>`)
2) Add AzDO target (`target add --provider azure-devops --scope <org> <project>`)
3) Store PAT in keyring (`token set --provider azure-devops --scope <org> <project> --token <pat>`)
4) Run sync (`sync`):
   - clones missing repos into <root>/azure-devops/<org>/<project>/<repo>
   - re-run sync:
     - if clean: fast-forward only
     - if dirty: skip modifying working tree
