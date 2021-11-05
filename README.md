# gitlab-ci-crawler

This crawler will go through all GitLab projects known to the person using it and build
a dependency graph inside a Neo4j database answering questions for template maintainers
who is pulling their templates on what version.

# Getting started

Prerequisites:

* Neo4j database with user
* A GitLab access token with `read_repository` permissions for all projects you want to crawl

Then you can run the code like:

```shell
 export GITLAB_TOKEN='<personal-access-token>'
 export NEO4J_PASSWORD='<neo4j-password>'
gitlab-ci-crawler --gitlab-host https://gitlab.com --neo4j-host 'bolt://127.0.0.1:7687' --neo4j-username neo4j
```

Find the full help using `gitlab-ci-crawler --help`
