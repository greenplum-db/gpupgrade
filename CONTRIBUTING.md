# Contributing

We warmly welcome and appreciate contributions from the community! By participating you agree to the [code of conduct](https://github.com/greenplum-db/gpupgrade/blob/master/CODE-OF-CONDUCT.md). To contribute:

- Sign our [Contributor License Agreement](https://cla.pivotal.io/sign/greenplum).

- Fork the gpupgrade repository on GitHub.

- Clone the repository.

- Follow the README to set up your environment and run the tests.

- Create a change

    - Create a topic branch.

    - Make commits as logical units for ease of reviewing.

    - Try and follow similar coding styles as found throughout the code base.

    - Rebase with master often to stay in sync with upstream.

    - Add appropriate tests and view coverage with `make coverage`.

    - Ensure a well written commit message as explained [here](https://chris.beams.io/posts/git-commit/) and [here](https://tbaggery.com/2008/04/19/a-note-about-git-commit-messages.html).

    - Format code with `make format`.
     
    - Address any linter issues with `make lint`.

- Submit a pull request (PR).

    - Create a [pull request from your fork](https://help.github.com/en/github/collaborating-with-issues-and-pull-requests/.creating-a-pull-request-from-a-fork).

    - We will create a test pipeline which runs additional acceptance tests based on your branch.

    - Address PR feedback with fixup and/or squash commits.
        ```
        git add .
        git commit --fixup <commit SHA> 
            Or
        git commit --squash <commit SHA>
        ```    

    - Once approved, before merging into master squash your fixups with:
        ```
        git rebase -i --autosquash origin/master
        git push --force-with-lease $USER <my-feature-branch>
        ```


# Community

Connect with Greenplum on:
- [Slack](https://greenplum.slack.com/)
- [Dev Google Group mailing list](https://groups.google.com/a/greenplum.org/forum/#!forum/gpdb-dev/join)

