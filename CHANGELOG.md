# buildevents changelog

## v0.17.0 - 2024-10-16

### Fixes

- fix(watch): not_running is the new queued (#221) | [Liz Fong-Jones](https://github.com/lizthegrey)

### Maintenance

- maint: update go version and docs (#223) | [Jamie Danielson](https://github.com/JamieDanielson)
- maint(go): update to go1.23 (#222) | [Liz Fong-Jones](https://github.com/lizthegrey)
- maint(deps): bump the examples group with 3 updates (#218) | [dependabot[bot]](https://github.com/dependabot[bot])
- docs: update vulnerability reporting process (#219) | [Robb Kidd](https://github.com/robbkidd)
- maint: update repo for pipeline team ownership (#217) | [Jamie Danielson](https://github.com/JamieDanielson)
- maint(deps): bump github.com/stretchr/testify from 1.8.4 to 1.9.0 (#214) | [dependabot[bot]](https://github.com/dependabot[bot])
- maint(deps): bump github.com/honeycombio/beeline-go from 1.14.0 to 1.15.0 (#212) | [dependabot[bot]](https://github.com/dependabot[bot])

## 0.16.0 - 2024-02-29

### Enhancements

- upgrade to libhoney-go 1.21.0 to support classic ingest keys (#208) | [@cewkrupa](https://github.com/cewkrupa)

### Maintenance

- Bump github.com/honeycombio/beeline-go from 1.13.0 to 1.14.0 (#206) | [dependabot](https://github.com/dependabot)
- Bump github.com/spf13/cobra from 1.7.0 to 1.8.0 (#205) | [dependabot](https://github.com/dependabot)
- Bump github.com/honeycombio/beeline-go from 1.12.0 to 1.13.0 (#202) | [dependabot](https://github.com/dependabot)

## 0.15.0 - 2023-07-17

### Enhancements

- add `meta.arch` and `meta.os` fields to spans (#200) | [@ismith](https://github.com/ismith)

### Maintenance

- Bump github.com/honeycombio/beeline-go from 1.11.1 to 1.12.0 (#198) | [dependabot](https://github.com/dependabot)
- Bump github.com/honeycombio/libhoney-go from 1.19.0 to 1.20.0 (#199) | [dependabot](https://github.com/dependabot)

## 0.14.0 - 2023-06-23

### Enhancements

- add support for Windows artifacts (#193) | [@jharley](https://github.com/jharley)

### Maintenance

- Bump Go to 1.20 (#193) | [@jharley](https://github.com/jharley)
- Bump github.com/spf13/cobra from 1.6.1 to 1.7.0 (#191) | [dependabot](https://github.com/dependabot)
- Bump github.com/stretchr/testify from 1.8.0 to 1.8.4 (#178, #187, #192) | [dependabot](https://github.com/dependabot)
- Bump github.com/honeycombio/beeline-go from 1.9.0 to 1.11.1 (#170, #179) | [dependabot](https://github.com/dependabot)
- Bump github.com/honeycombio/libhoney-go from 1.16.0 to 1.18.0 (#180) | [dependabot](https://github.com/dependabot)
- Bump github.com/spf13/cobra from 1.5.0 to 1.7.0 (#181, #191) | [dependabot](https://github.com/dependabot)

## 0.13.0 - 2022-08-25

### Enhancements

- include sending error true for spans representing error states for builds (#168) | [@mjayaram](https://github.com/mjayaram)
- Removes checking and warning for when only the dataset is configured for Buildevents; Updates logic to set default is no Dataset is configured for the non-classic use (#167) | [@mjayaram](https://github.com/mjayaram)

### Maintenance

- Bump github.com/honeycombio/beeline-go from 1.8.0 to 1.9.0 (#166) | [dependabot](https://github.com/dependabot)

## 0.12.1 - 2022-07-20

### Maintenance

- Re-release to fix OpenSSL CVE | [@kentquirk](https://github.com/kentquirk)

## 0.12.0 - 2022-07-14

### Enhancements

- Add support to send data to environments (#162) | [@MikeGoldsmith](https://github.com/MikeGoldsmith)

## 0.11.0 - 2022-07-13

### Enhancements

- add gha-buildevents as GHA provider alias (#160) | [@dstrelau](https://github.com/dstrelau)

### Maintenance

- Bump github.com/spf13/cobra from 1.4.0 to 1.5.0 (#161) | [dependabot](https://github.com/dependabot)

## 0.10.0 - 2022-06-14

### Enhancements

- Build for Darwin ARM64 (#157) | [Kent Quirk](https://github.com/kentquirk) & [John Dorman](https://github.com/boostchicken)

### Maintenance

- [docs] Add examples of generated events (#155) | [Kent Quirk](https://github.com/kentquirk)
- [docs] Remember to update orb when releasing a new version (#152) | [Vera Reynolds](https://github.com/vreynolds)

## 0.9.2 - 2022-04-25

### Maintenance

- update ci image to cimg/go:1.18 (#150) | [@JamieDanielson](https://github.com/JamieDanielson)
  - - fixes openSSL CVE

## 0.9.1 - 2022-04-15

- [bug] Fix default value for dataset to be empty so that dataset determination logic works correctly. (#148) [@kentquirk](https://github.com/kentquirk)

## 0.9.0 - 2022-04-14

- Bump cobra to v1.4.0
- Bump beeline to v1.8.0
- Bump libhoney to v1.15.8
- Use cobra.MatchAll instead of identical custom code
- Clean up buildURL function to construct URLs more safely
- The `service_name` field is mirrored to `service.name`
- Detect classic key and change behavior for non-classic mode:
  - Service Name, if specified, is used as the dataset as well as both `service_name` and `service.name` fields.
  - If dataset is specified and service name is not, it will be used but will generate a warning (except in quiet mode).
  - If both are specified, service name will be used, dataset is ignored, and a warning will be emitted (except in quiet mode).
  - The command name is now sent as command_name (in classic it is still sent as service_name).
  - The `watch` command now sets the `name` field to merely `watch` rather than a high-cardinality value, making it easier to aggregate queries across different builds.
  - Dataset name is trimmed of leading/trailing whitespace; if any was found emits a warning (except in quiet mode)

## 0.8.0 - 2022-01-13

### Fixes

- Return underlying exit code when running commands (#137) | [@jhchabran](https://github.com/jhchabran)

## 0.7.2 - 2022-01-07

### Fixes

- Display underlying error when verifying API key (#135) | [@jhchabran](https://github.com/jhchabran)

### Maintenance

- Update ci image (#132) | [@vreynolds](https://github.com/vreynolds)
- Add re-triage workflow (#131) | [@vreynolds](https://github.com/vreynolds)
- Only create one draft gh release (#128) | [@vreynolds](https://github.com/vreynolds)
- Bump github.com/spf13/cobra from 0.0.7 to 1.2.1 (#130)
- Bump github.com/honeycombio/beeline-go from 1.3.1 to 1.3.2 (#129)
- Bump github.com/honeycombio/beeline-go from 1.2.0 to 1.3.1 (#123)

## 0.7.1 - 2021-11-19

### Fixed

- Do not fail the build if `watch` fails to fetch Honeycomb URL (#126) | [@asdvalenzuela](https://github.com/asdvalenzuela)

### Maintenance

- Create draft gh release during publish (#124) | [@MikeGoldsmith](https://github.com/MikeGoldsmith)

## 0.7.0 - 2021-11-03

### Added

- Allow specifying an alternative shell (#119) | [@estheruary](https://github.com/estheruary)

### Maintenance

- empower apply-labels action to apply labels (#120)
- bump libhoney-go to v1.15.6 (#121)
- Bump github.com/honeycombio/libhoney-go from 1.15.4 to 1.15.5 (#118)
- Change maintenance badge to maintained (#116)
- Adds Stalebot (#117)
- Add NOTICE (#113)
- Bump github.com/honeycombio/beeline-go from 1.1.2 to 1.2.0 (#109)
- Bump github.com/honeycombio/libhoney-go from 1.15.3 to 1.15.4 (#108)
- Add issue and PR templates (#112)
- Add OSS lifecycle badge (#111)
- Add community health files (#110)

## 0.6.0 - 2021-07-14

### Added

- Forward stdin. [#99](https://github.com/honeycombio/buildevents/pull/99) | [@shlevy](https://github.com/shlevy)

### Maintenance

- Bump github.com/spf13/cobra from 0.0.5 to 0.0.7 [#102](https://github.com/honeycombio/buildevents/pull/102)
- Bump github.com/honeycombio/libhoney-go from 1.10.0 to 1.15.3 [#101](https://github.com/honeycombio/buildevents/pull/101)
- Bump github.com/jszwedko/go-circleci from 0.2.0 to 0.3.0 [#103](https://github.com/honeycombio/buildevents/pull/103)
- stop watching dependabot builds [#106](https://github.com/honeycombio/buildevents/pull/106)

## 0.5.2 - 2021-07-08

### Added

- Add support for Buildkite CI environment detection. [#97](https://github.com/honeycombio/buildevents/pull/97) | [@MikeGoldsmith](https://github.com/MikeGoldsmith)

## 0.5.1 - 2021-03-27

### Added

- Added ARM64 builds. [#91](https://github.com/honeycombio/buildevents/pull/91) | [@ismith](https://github.com/ismith)

## 0.5.0 - 2021-02-09

### Added

- Quiet option to cmd [#80](https://github.com/honeycombio/buildevents/pull/80) | [@tybritten](https://github.com/tybritten)
- Bitbucket support [#85](https://github.com/honeycombio/buildevents/pull/85) | [@manjunathb4461](https://github.com/manjunathb4461)
- Support for overriding default event fields [#76](https://github.com/honeycombio/buildevents/pull/76) | [@MarilynFranklin](https://github.com/MarilynFranklin)

### Fixed

- Azure pipelines constant typo [#84](https://github.com/honeycombio/buildevents/pull/84) | [@manjunathb4461](https://github.com/manjunathb4461)
