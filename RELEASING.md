# Releasing

- Use [go-licenses](https://github.com/google/go-licenses) to ensure all project dependency licenses are correctly represented in this repository:
  - Install go-licenses (if not already installed) `go install github.com/google/go-licenses@latest`
  - Run and save licenses `go-licenses save github.com/honeycombio/buildevents --save_path="./LICENSES"`
  - If there are any changes, submit a separate PR to update licenses.
- Prep update PR for the [orb](https://github.com/honeycombio/buildevents-orb) with the new version of buildevents.
- Update `CHANGELOG.md` with the changes since the last release. Consider automating with a command such as these two:
  - `git log $(git describe --tags --abbrev=0)..HEAD --no-merges --oneline > new-in-this-release.log`
  - `git log --pretty='%C(green)%d%Creset- %s | [%an](https://github.com/)'`
- Commit changes, push, and open a release preparation pull request for review.
- Once the pull request is merged, fetch the updated `main` branch.
- Apply a tag for the new version on the merged commit (e.g. `git tag -a v2.3.1 -m "v2.3.1"`)
- Push the tag upstream (this will kick off the release pipeline in CI) e.g. `git push origin v2.3.1`
- Ensure that there is a draft GitHub release created as part of CI publish steps.
- Click "generate release notes" in GitHub for full changelog notes and any new contributors.
