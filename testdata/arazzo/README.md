# testdata/arazzo

Fixtures for FileLoader, catalog, and engine tests. `FileLoader` must be pointed at **`plans/`**, not this directory (otherwise `sources/openapi.yaml` is parsed as Arazzo and fails).

```text
testdata/arazzo/
  plans/
    petstore-v1.0.0.yaml   x-planId: petstore, version 1.0.0, workflow pingHealth
    petstore-v1.1.0.yaml   same planId, version 1.1.0, pingHealth + echoName
    no-plan-id.yaml        skipped (no x-planId)
    ignore.txt             skipped (not yaml/json)
  sources/
    openapi.yaml           OpenAPI 3; operationId getHealth
                           referenced from plans as ../sources/openapi.yaml
```

Latest petstore version in tests is `1.1.0`.
