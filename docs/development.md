# Development

Requirements: Go 1.24 or newer.

```sh
make test
make lint
make build
```

Use `net/http/httptest` for API tests. Keep Bubble Tea models free of direct HTTP concerns and add behavior tests beside the package under test.
