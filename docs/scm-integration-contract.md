# SCM Integration Contract

Git enriches Atlas. Git does not define Atlas truth.

## Rules
- absence of a git repo never breaks Atlas core behavior
- branch names are advisory
- commit parsing adds refs and context only
- ticket workflow state is still owned by Atlas commands and services
- GitHub CLI integration is optional and may fail independently of Atlas core behavior
