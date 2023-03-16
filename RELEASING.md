# Creating a new release

1. Use [go-licenses](https://github.com/google/go-licenses) to ensure all project dependency licenses are correclty represented in this repository:
  1. Install go-licenses (if not already installed) `go install github.com/google/go-licenses@latest`
  2. Run and save liceses `go-licenses save github.com/honeycombio/buildevents --save_path="./LICENSES"`
  3. If there are any changes, submit PR to update licneses.
2. Prep update PR for the [orb](https://github.com/honeycombio/buildevents-orb) with the new version of buildevents.
3. Add new entry in the CHANGELOG.
4. Once the above change is merged into `main`, tag `main` with the new version, e.g. `v0.6.1`. Push the tags. This will kick off CI, which will create a draft GitHub release.
5. Update release notes using the CHANGELOG entry on the new draft GitHub release, and publish it.
