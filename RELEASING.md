# Releasing

- Check that licenses are current with `make verify-licenses`
  - If there are any changes, submit a separate PR to update licenses using `make update-licenses`.
- Prep update PR for the [orb](https://github.com/honeycombio/buildevents-orb) with the new version of buildevents.
- Update `CHANGELOG.md` with the changes since the last release.
  - Use below command to get a list of all commits since last release

    ```sh
    git log <last-release-tag>..HEAD --pretty='%Creset- %s | [%an](https://github.com/%an)'
    ```

  - Copy the output from the command above into the top of [changelog](./CHANGELOG.md)
  - fix each `https://github.com/<author-name>` to point to the correct github username
  (the `git log` command can't do this automatically)
  - organize each commit based on their prefix into below three categories:

    ```sh
    ### Features
      - <a-commit-with-feat-prefix>

    ### Fixes
      - <a-commit-with-fix-prefix>

    ### Maintenance
      - <a-commit-with-maintenance-prefix>
    ```

- Commit changes, push, and open a release preparation pull request for review.
- Once the pull request is merged, fetch the updated `main` branch.
- Apply a tag for the new version on the merged commit (e.g. `git tag -a v2.3.1 -m "v2.3.1"`)
- Push the tag upstream (this will kick off the release pipeline in CI) e.g. `git push origin v2.3.1`
- Ensure that there is a draft GitHub release created as part of CI publish steps.
- Click "generate release notes" in GitHub for full changelog notes and any new contributors.
