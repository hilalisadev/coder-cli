# ci

## steps
- `steps/build.sh` builds release artifacts with the appropriate release tag.
It is required for merging as it ensures that build artifacts can be produced from source.
- `steps/fmt.sh` checks that all Go code is properly formatted.
- `steps/lint.sh` checks that the `.golangci.yml` rules pass successfully.
- `steps/gendocs.sh` auto-generates the CLI documentation in `/docs` and ensures it is up-to-date.

## integration tests

### `tcli`

Package `tcli` provides a framework for writing end-to-end CLI tests.
Each test group can have its own container for executing commands in a consistent
and isolated filesystem.

### prerequisites

Assign the following environment variables to run the integration tests
against an existing Enterprise deployment instance.

```bash
export CODER_URL=...
export CODER_EMAIL=...
export CODER_PASSWORD=...
```

Then, simply run the test command from the project root

```sh
go test -v ./ci/integration
```
