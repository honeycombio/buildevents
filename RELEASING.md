# Creating a new release

1. Prep update PR for the [orb](https://github.com/honeycombio/buildevents-orb) with the new version of buildevents.
2. Add new entry in the CHANGELOG.
3. Once the above change is merged into `main`, tag `main` with the new version, e.g. `v0.6.1`. Push the tags. This will kick off CI, which will create a draft GitHub release.
4. Update release notes using the CHANGELOG entry on the new draft GitHub release, and publish it.
