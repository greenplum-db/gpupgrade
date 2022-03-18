# Contributing

We warmly welcome and appreciate contributions from the community! By participating you agree to the [code of conduct](https://github.com/greenplum-db/gpupgrade/blob/master/CODE-OF-CONDUCT.md).

## Development
- Gather input early and often rather than waiting until the end. 
  - Have in-person conversations.
  - Regularly share your branch of work.
  - Consider making a draft PR.
  - Pair as needed.
- Prefer short names based on context such as: file vs. database_file
  - People will be familiar with the code, so err on the side of brevity but avoid extremes.
- Generally follow surrounding code style and conventions. Use `make lint`.
- Have tests including unit and end-to-end.
- Resources:
  - [Effective Go](https://golang.org/doc/effective_go.html)
  - [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
  - [Practical Go](https://dave.cheney.net/practical-go/presentations/qcon-china.html)
  - [The Go Memory Model](https://go.dev/ref/mem)
  - [Protocol Buffers Syle Guide](https://developers.google.com/protocol-buffers/docs/style)
  - [BASH Style Guide](https://google.github.io/styleguide/shellguide.html)
  - [Refactoring by Martin Fowler](https://martinfowler.com/books/refactoring.html) including the [Refactoring website](https://refactoring.com/). 

## Structuring PRs
Optimize the review experience for others. The goal is to make reviewing PRs easy for others. [Example PR](https://github.com/greenplum-db/gpdb/pull/12573).
- Ensure PRs are single focused. If more work is needed specify followup PRs, as multiple PRs per feature is fine.
- Avoid one large PR that is hard to review. Consider multiple smaller scoped PRs.
- Ensure PRs are completed before submitting.
  - Ensure all unit and end-to-end tests are written.
  - A successful test pipeline has been run.
  - Make a draft PR for early feedback.
- Ensure the PR description is detailed.
  - Include a link to the test pipeline for others to review.
- Create individual logical commits with detailed descriptions.
  - Try to keep pre-factors and code motion to their own commits.
- Minimize lots of commits after submitting.
- Minimize morphing PR branches into WIP branches.
- Only merge PRs once at least one other person who did not work on the code approves it.
- Squash all relevant commits and do a "Rebase and merge". Do not perform a "merge commit" or "squash and merge".

## Contributing

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

## Code Reviews
- Follow [Google's Code Review Guidelines](https://google.github.io/eng-practices/review/reviewer/)
- PR comments should have technical explanations.
- Avoid “I prefer it this way”. See [Principles Section](https://google.github.io/eng-practices/review/reviewer/standard.html).
- Avoid these [Toxic Behaviors](https://medium.com/@sandya.sankarram/unlearning-toxic-behaviors-in-a-code-review-culture-b7c295452a3c) ([video](https://www.youtube.com/watch?v=QIUwGa-MttQ))
- Use Github's "Request changes" very sparingly. This indicates that there are critical blockers that absolutely must change before approval.
- Use Github's “Start a review” feature to submit multiple comments into a single review.
- Address PR comments with fixup or squash commits. This makes it easier for the review to see what changed.
  - Ideally wait until the PR has been approved to squash tehse commits, but sometimes it might be cleaner and easier to follow to combine them earlier.
  - Rebasing your PR with master is good practice.
- Use Github’s “Resolve Conversation” button to indicate you addressed the feedback. There is no need for a comment unless you deviated from the reviewer's specific feedback.
- Tag an individual person if you want a review from them, otherwise tag the "Upgrade" team. If you want a re-review, tag a specific person in a comment to send them a notification.

# Community

Connect with Greenplum on:
- [Slack](https://greenplum.slack.com/)
- [gpdb-dev mailing list](https://groups.google.com/a/greenplum.org/forum/#!forum/gpdb-dev/join)

